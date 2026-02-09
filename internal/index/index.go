package index

import (
	"sort"
	"strings"
	"time"

	"github.com/armash/log-pipeline/internal/types"
)

type Index struct {
	ByLevel map[string][]types.LogEntry
	ByHour  map[string][]types.LogEntry
	Hours   []string
}

// Build creates in-memory indexes by level and hour bucket.
func Build(entries []types.LogEntry) *Index {
	idx := &Index{
		ByLevel: make(map[string][]types.LogEntry),
		ByHour:  make(map[string][]types.LogEntry),
	}

	for _, e := range entries {
		levelKey := strings.ToUpper(e.Level)
		idx.ByLevel[levelKey] = append(idx.ByLevel[levelKey], e)

		hourKey := hourBucket(e.Timestamp)
		idx.ByHour[hourKey] = append(idx.ByHour[hourKey], e)
	}

	idx.Hours = make([]string, 0, len(idx.ByHour))
	for key := range idx.ByHour {
		idx.Hours = append(idx.Hours, key)
	}
	sort.Strings(idx.Hours)

	return idx
}

// Filter returns entries matching the filters using indexes when available.
func Filter(all []types.LogEntry, idx *Index, level string, cutoff time.Time, search string) []types.LogEntry {
	candidates := all
	if idx != nil {
		if level != "" {
			levelKey := strings.ToUpper(level)
			candidates = idx.ByLevel[levelKey]
		} else if !cutoff.IsZero() {
			candidates = collectFromHourBuckets(idx, cutoff)
		}
	}

	filtered := make([]types.LogEntry, 0, len(candidates))
	for _, e := range candidates {
		if level != "" && !strings.EqualFold(e.Level, level) {
			continue
		}
		if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(search)) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

func collectFromHourBuckets(idx *Index, cutoff time.Time) []types.LogEntry {
	if idx == nil || len(idx.Hours) == 0 {
		return nil
	}

	startKey := hourBucket(cutoff)
	out := make([]types.LogEntry, 0)
	for _, key := range idx.Hours {
		if key < startKey {
			continue
		}
		out = append(out, idx.ByHour[key]...)
	}
	return out
}

func hourBucket(t time.Time) string {
	return t.UTC().Format("2006-01-02T15")
}
