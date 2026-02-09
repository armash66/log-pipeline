package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/armash/log-pipeline/internal/engine"
	"github.com/armash/log-pipeline/internal/index"
	"github.com/armash/log-pipeline/internal/query"
	"github.com/armash/log-pipeline/internal/types"
)

type Server struct {
	mu         sync.RWMutex
	entries    []types.LogEntry
	loadStats  engine.LoadStats
	useIndex   bool
	baseIndex  *index.Index
	lastMetric engine.Metrics
	hasMetric  bool
	storePath  string
	shardDir   string
	apiKey     string
}

func New(entries []types.LogEntry, stats engine.LoadStats, useIndex bool, baseIndex *index.Index, storePath string, shardDir string, apiKey string) *Server {
	return &Server{
		entries:   entries,
		loadStats: stats,
		useIndex:  useIndex,
		baseIndex: baseIndex,
		storePath: storePath,
		shardDir:  shardDir,
		apiKey:    apiKey,
	}
}

func (s *Server) Start(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/query", s.handleQuery)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/ingest", s.handleIngest)
	mux.HandleFunc("/", s.handleRoot)
	mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(webDir()))))

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/ui/", http.StatusFound)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	search := r.URL.Query().Get("search")
	since := r.URL.Query().Get("since")
	after := r.URL.Query().Get("after")
	before := r.URL.Query().Get("before")
	limitStr := r.URL.Query().Get("limit")
	q := r.URL.Query().Get("q")

	var cutoff time.Time
	if since != "" {
		d, err := time.ParseDuration(since)
		if err != nil {
			http.Error(w, "invalid since duration", http.StatusBadRequest)
			return
		}
		cutoff = time.Now().Add(-d)
	}

	filters := query.BuildFilters(level, cutoff, search)
	if after != "" {
		tm, err := time.Parse(time.RFC3339, after)
		if err != nil {
			http.Error(w, "invalid after timestamp", http.StatusBadRequest)
			return
		}
		filters.After = tm
	}
	if before != "" {
		tm, err := time.Parse(time.RFC3339, before)
		if err != nil {
			http.Error(w, "invalid before timestamp", http.StatusBadRequest)
			return
		}
		filters.Before = tm
	}
	if q != "" {
		parsed, err := query.Parse(q)
		if err != nil {
			http.Error(w, "invalid query", http.StatusBadRequest)
			return
		}
		merged, err := query.MergeFilters(filters, parsed)
		if err != nil {
			http.Error(w, "conflicting query filters", http.StatusBadRequest)
			return
		}
		filters = merged
	}

	limit := 0
	if limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = n
	}

	s.mu.RLock()
	entries := s.entries
	stats := s.loadStats
	useIndex := s.useIndex
	baseIndex := s.baseIndex
	s.mu.RUnlock()

	results, metrics := engine.QueryEntries(entries, stats, engine.QueryOptions{
		Filters:  filters,
		UseIndex: useIndex,
		Limit:    limit,
		Index:    baseIndex,
	})

	s.mu.Lock()
	s.lastMetric = metrics
	s.hasMetric = true
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(results),
		"logs":  results,
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	metrics := s.lastMetric
	hasMetric := s.hasMetric
	stats := s.loadStats
	useIndex := s.useIndex
	s.mu.RUnlock()

	if !hasMetric {
		metrics = engine.Metrics{
			StartedAt:       time.Now(),
			FinishedAt:      time.Now(),
			LogsRead:        stats.LogsRead,
			LogsIngested:    stats.LogsIngested,
			LogsFilteredOut: 0,
			LogsReturned:    stats.LogsIngested,
			IndexEnabled:    useIndex,
		}
	}

	writeJSON(w, http.StatusOK, metricsToMap(metrics))
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.apiKey != "" {
		if r.Header.Get("X-API-Key") != s.apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	var payload ingestPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	var entries []types.LogEntry
	if len(payload.Entries) > 0 {
		for _, item := range payload.Entries {
			entry, err := item.toEntry()
			if err != nil {
				http.Error(w, "invalid entry", http.StatusBadRequest)
				return
			}
			entries = append(entries, entry)
		}
	} else if payload.Entry != nil {
		entry, err := payload.Entry.toEntry()
		if err != nil {
			http.Error(w, "invalid entry", http.StatusBadRequest)
			return
		}
		entries = append(entries, entry)
	} else {
		http.Error(w, "missing entry", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	combined, stats, err := engine.IngestEntries(s.entries, entries, s.storePath, s.shardDir, "")
	if err != nil {
		s.mu.Unlock()
		http.Error(w, "failed to ingest", http.StatusInternalServerError)
		return
	}
	s.entries = combined
	s.loadStats.LogsRead += stats.LogsIngested
	s.loadStats.LogsIngested += stats.LogsIngested
	s.baseIndex = nil
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ingested": len(entries),
	})
}

func metricsToMap(m engine.Metrics) map[string]interface{} {
	rate, ok := m.RatePerSec()
	rateText := "NA"
	if ok {
		rateText = formatRate(rate)
	}
	return map[string]interface{}{
		"metrics.started_at":        m.StartedAt.UTC().Format(time.RFC3339),
		"metrics.finished_at":       m.FinishedAt.UTC().Format(time.RFC3339),
		"metrics.duration_ms":       m.Duration().Milliseconds(),
		"metrics.logs_read":         m.LogsRead,
		"metrics.logs_ingested":     m.LogsIngested,
		"metrics.logs_filtered_out": m.LogsFilteredOut,
		"metrics.logs_returned":     m.LogsReturned,
		"metrics.rate_per_sec":      rateText,
		"metrics.index_enabled":     m.IndexEnabled,
	}
}

func formatRate(val float64) string {
	return strconv.FormatFloat(val, 'f', 2, 64)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func webDir() string {
	return filepath.Join("web")
}

type ingestPayload struct {
	Entry   *ingestEntry   `json:"entry"`
	Entries []ingestEntry  `json:"entries"`
}

type ingestEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func (e ingestEntry) toEntry() (types.LogEntry, error) {
	if e.Timestamp == "" || e.Level == "" || e.Message == "" {
		return types.LogEntry{}, fmt.Errorf("missing fields")
	}
	t, err := time.Parse(time.RFC3339, e.Timestamp)
	if err != nil {
		return types.LogEntry{}, err
	}
	return types.LogEntry{
		Timestamp: t,
		Level:     e.Level,
		Message:   e.Message,
	}, nil
}
