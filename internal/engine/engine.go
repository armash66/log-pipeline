package engine

import (
	"time"

	"github.com/armash/log-pipeline/internal/index"
	"github.com/armash/log-pipeline/internal/ingest"
	"github.com/armash/log-pipeline/internal/query"
	"github.com/armash/log-pipeline/internal/store"
	"github.com/armash/log-pipeline/internal/types"
)

type LoadOptions struct {
	File            string
	Format          ingest.Format
	LoadPath        string
	StorePath       string
	Replay          bool
	Retention       time.Duration
	StoreHeaderText string
}

type LoadStats struct {
	LogsRead     int
	LogsIngested int
}

type QueryOptions struct {
	Filters  query.Filters
	UseIndex bool
	Limit    int
}

type Metrics struct {
	StartedAt      time.Time
	FinishedAt     time.Time
	LogsRead       int
	LogsIngested   int
	LogsFilteredOut int
	LogsReturned   int
	IndexEnabled   bool
}

func (m Metrics) Duration() time.Duration {
	return m.FinishedAt.Sub(m.StartedAt)
}

func (m Metrics) RatePerSec() (float64, bool) {
	secs := m.Duration().Seconds()
	if secs < 1 {
		return 0, false
	}
	return float64(m.LogsIngested) / secs, true
}

// LoadEntries loads entries from a file or JSONL store and optionally appends to a store.
func LoadEntries(opts LoadOptions) ([]types.LogEntry, LoadStats, error) {
	var entries []types.LogEntry
	stats := LoadStats{}

	if opts.LoadPath != "" {
		loaded, err := store.LoadJSONL(opts.LoadPath)
		if err != nil {
			return nil, stats, err
		}
		entries = append(entries, loaded...)
		stats.LogsRead = len(loaded)
		stats.LogsIngested = len(loaded)
	} else {
		if opts.Replay && opts.StorePath != "" {
			loaded, err := store.LoadJSONL(opts.StorePath)
			if err != nil {
				return nil, stats, err
			}
			entries = append(entries, loaded...)
		}

		newEntries, err := ingest.ReadLogFileWithFormat(opts.File, opts.Format)
		if err != nil {
			return nil, stats, err
		}
		entries = append(entries, newEntries...)
		stats.LogsRead = len(newEntries)
		stats.LogsIngested = len(newEntries)

		if opts.StorePath != "" {
			if opts.StoreHeaderText != "" {
				if err := store.AppendHeader(opts.StorePath, opts.StoreHeaderText); err != nil {
					return nil, stats, err
				}
			}
			if err := store.AppendJSONL(opts.StorePath, newEntries); err != nil {
				return nil, stats, err
			}
		}
	}

	if opts.Retention > 0 {
		cutoff := time.Now().Add(-opts.Retention)
		entries = applyRetention(entries, cutoff)
	}

	return entries, stats, nil
}

// QueryEntries filters entries and returns results with metrics.
func QueryEntries(entries []types.LogEntry, loadStats LoadStats, opts QueryOptions) ([]types.LogEntry, Metrics) {
	start := time.Now()
	var filtered []types.LogEntry
	if opts.UseIndex {
		idx := index.Build(entries)
		filtered = index.FilterWithFilters(entries, idx, opts.Filters)
	} else {
		filtered = make([]types.LogEntry, 0, len(entries))
		for _, e := range entries {
			if !query.MatchesFilters(e, opts.Filters) {
				continue
			}
			filtered = append(filtered, e)
		}
	}

	limited := filtered
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		limited = filtered[:opts.Limit]
	}

	metrics := Metrics{
		StartedAt:       start,
		FinishedAt:      time.Now(),
		LogsRead:        loadStats.LogsRead,
		LogsIngested:    loadStats.LogsIngested,
		LogsFilteredOut: len(entries) - len(filtered),
		LogsReturned:    len(limited),
		IndexEnabled:    opts.UseIndex,
	}

	return limited, metrics
}

func applyRetention(entries []types.LogEntry, cutoff time.Time) []types.LogEntry {
	filtered := make([]types.LogEntry, 0, len(entries))
	for _, e := range entries {
		if e.Timestamp.Before(cutoff) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}
