package server

import (
	"context"
	"encoding/json"
	"net/http"
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
}

func New(entries []types.LogEntry, stats engine.LoadStats, useIndex bool, baseIndex *index.Index) *Server {
	return &Server{
		entries:   entries,
		loadStats: stats,
		useIndex:  useIndex,
		baseIndex: baseIndex,
	}
}

func (s *Server) Start(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/query", s.handleQuery)
	mux.HandleFunc("/metrics", s.handleMetrics)

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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	search := r.URL.Query().Get("search")
	since := r.URL.Query().Get("since")
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
