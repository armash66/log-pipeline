package engine

import (
	"fmt"
	"time"

	"github.com/armash/log-pipeline/internal/index"
	"github.com/armash/log-pipeline/internal/ingest"
	"github.com/armash/log-pipeline/internal/query"
	"github.com/armash/log-pipeline/internal/snapshot"
	"github.com/armash/log-pipeline/internal/store"
	"github.com/armash/log-pipeline/internal/types"
)

type LoadOptions struct {
	File            string
	Format          ingest.Format
	LoadPath        string
	StorePath       string
	SnapshotPath    string
	ShardDir        string
	ShardPaths      []string
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
	Index    *index.Index
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
type LoadResult struct {
	Entries []types.LogEntry
	Stats   LoadStats
	Index   *index.Index
}

type IngestStats struct {
	LogsIngested int
}

func LoadEntries(opts LoadOptions) (LoadResult, error) {
	var entries []types.LogEntry
	stats := LoadStats{}
	var loadedIndex *index.Index

	if opts.SnapshotPath != "" {
		snap, err := snapshot.Load(opts.SnapshotPath)
		if err != nil {
			return LoadResult{}, err
		}
		if snap.Metadata.Version != snapshot.Version {
			return LoadResult{}, fmt.Errorf("snapshot version mismatch")
		}
		entries = append(entries, snap.Entries...)
		stats.LogsRead = len(snap.Entries)
		stats.LogsIngested = len(snap.Entries)
		loadedIndex = index.FromSnapshotIndex(snap.Index, snap.Entries)

		if opts.Replay && opts.StorePath != "" {
			loaded, err := store.LoadJSONL(opts.StorePath)
			if err != nil {
				return LoadResult{}, err
			}
			entries = append(entries, loaded...)
			stats.LogsRead += len(loaded)
			stats.LogsIngested += len(loaded)
			loadedIndex = nil
		}
	} else if opts.LoadPath != "" {
		loaded, err := store.LoadJSONL(opts.LoadPath)
		if err != nil {
			return LoadResult{}, err
		}
		entries = append(entries, loaded...)
		stats.LogsRead = len(loaded)
		stats.LogsIngested = len(loaded)
	} else if len(opts.ShardPaths) > 0 {
		loaded, err := store.LoadJSONLFromMany(opts.ShardPaths)
		if err != nil {
			return LoadResult{}, err
		}
		entries = append(entries, loaded...)
		stats.LogsRead = len(loaded)
		stats.LogsIngested = len(loaded)
	} else {
		if opts.Replay && opts.StorePath != "" {
			loaded, err := store.LoadJSONL(opts.StorePath)
			if err != nil {
				return LoadResult{}, err
			}
			entries = append(entries, loaded...)
		}

		newEntries, err := ingest.ReadLogFileWithFormat(opts.File, opts.Format)
		if err != nil {
			return LoadResult{}, err
		}
		entries = append(entries, newEntries...)
		stats.LogsRead = len(newEntries)
		stats.LogsIngested = len(newEntries)

		if opts.StorePath != "" {
			if opts.StoreHeaderText != "" {
				if err := store.AppendHeader(opts.StorePath, opts.StoreHeaderText); err != nil {
					return LoadResult{}, err
				}
			}
			if err := store.AppendJSONL(opts.StorePath, newEntries); err != nil {
				return LoadResult{}, err
			}
		}

		if opts.ShardDir != "" {
			if err := store.AppendShards(opts.ShardDir, newEntries); err != nil {
				return LoadResult{}, err
			}
		}
	}

	if opts.Retention > 0 {
		cutoff := time.Now().Add(-opts.Retention)
		entries = applyRetention(entries, cutoff)
	}

	return LoadResult{
		Entries: entries,
		Stats:   stats,
		Index:   loadedIndex,
	}, nil
}

// QueryEntries filters entries and returns results with metrics.
func QueryEntries(entries []types.LogEntry, loadStats LoadStats, opts QueryOptions) ([]types.LogEntry, Metrics) {
	start := time.Now()
	var filtered []types.LogEntry
	if opts.UseIndex {
		idx := opts.Index
		if idx == nil {
			idx = index.Build(entries)
		}
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

// IngestEntries appends entries to stores and shards, and returns updated entries slice.
func IngestEntries(existing []types.LogEntry, entries []types.LogEntry, storePath string, shardDir string, storeHeaderText string) ([]types.LogEntry, IngestStats, error) {
	stats := IngestStats{LogsIngested: len(entries)}
	if storePath != "" {
		if storeHeaderText != "" {
			if err := store.AppendHeader(storePath, storeHeaderText); err != nil {
				return existing, stats, err
			}
		}
		if err := store.AppendJSONL(storePath, entries); err != nil {
			return existing, stats, err
		}
	}
	if shardDir != "" {
		if err := store.AppendShards(shardDir, entries); err != nil {
			return existing, stats, err
		}
	}
	combined := append(existing, entries...)
	return combined, stats, nil
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
