package index

import (
	"sort"
	"strings"
	"time"

	"github.com/armash/log-pipeline/internal/query"
	"github.com/armash/log-pipeline/internal/types"
)

type Index struct {
	ByLevel map[string][]types.LogEntry
	ByHour  map[string][]types.LogEntry
	Hours   []string
}

// SnapshotIndex stores index buckets as entry indices for snapshot persistence.
type SnapshotIndex struct {
	ByLevel map[string][]int `json:"byLevel"`
	ByHour  map[string][]int `json:"byHour"`
	Hours   []string         `json:"hours"`
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

// ToSnapshotIndex converts an in-memory index into a snapshot-friendly index.
func ToSnapshotIndex(idx *Index, entries []types.LogEntry) SnapshotIndex {
	si := SnapshotIndex{
		ByLevel: make(map[string][]int),
		ByHour:  make(map[string][]int),
	}

	hourSet := make(map[string]struct{})
	for i, e := range entries {
		levelKey := strings.ToUpper(e.Level)
		si.ByLevel[levelKey] = append(si.ByLevel[levelKey], i)

		hourKey := hourBucket(e.Timestamp)
		si.ByHour[hourKey] = append(si.ByHour[hourKey], i)
		hourSet[hourKey] = struct{}{}
	}

	si.Hours = make([]string, 0, len(hourSet))
	for key := range hourSet {
		si.Hours = append(si.Hours, key)
	}
	sort.Strings(si.Hours)

	return si
}

// FromSnapshotIndex rebuilds an in-memory index from a snapshot index.
func FromSnapshotIndex(si SnapshotIndex, entries []types.LogEntry) *Index {
	idx := &Index{
		ByLevel: make(map[string][]types.LogEntry),
		ByHour:  make(map[string][]types.LogEntry),
		Hours:   append([]string(nil), si.Hours...),
	}

	for level, indices := range si.ByLevel {
		for _, i := range indices {
			if i >= 0 && i < len(entries) {
				idx.ByLevel[level] = append(idx.ByLevel[level], entries[i])
			}
		}
	}
	for hour, indices := range si.ByHour {
		for _, i := range indices {
			if i >= 0 && i < len(entries) {
				idx.ByHour[hour] = append(idx.ByHour[hour], entries[i])
			}
		}
	}

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

// FilterWithFilters returns entries matching query filters using indexes when available.
func FilterWithFilters(all []types.LogEntry, idx *Index, f query.Filters) []types.LogEntry {
	if len(f.Or) > 0 {
		combined := make([]types.LogEntry, 0)
		seen := make(map[string]struct{})
		for _, opt := range f.Or {
			part := FilterWithFilters(all, idx, opt)
			for _, e := range part {
				key := e.Timestamp.Format(time.RFC3339Nano) + "|" + e.Level + "|" + e.Message
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				combined = append(combined, e)
			}
		}
		return combined
	}

	candidates := all
	if idx != nil {
		if f.Level != "" {
			levelKey := strings.ToUpper(f.Level)
			candidates = idx.ByLevel[levelKey]
		} else if len(f.LevelIn) > 0 {
			union := make([]types.LogEntry, 0)
			seen := make(map[string]struct{})
			for _, lvl := range f.LevelIn {
				levelKey := strings.ToUpper(lvl)
				for _, e := range idx.ByLevel[levelKey] {
					key := e.Timestamp.Format(time.RFC3339Nano) + "|" + e.Level + "|" + e.Message
					if _, ok := seen[key]; ok {
						continue
					}
					seen[key] = struct{}{}
					union = append(union, e)
				}
			}
			candidates = union
		} else if !f.After.IsZero() {
			candidates = collectFromHourBuckets(idx, f.After)
		}
	}

	filtered := make([]types.LogEntry, 0, len(candidates))
	for _, e := range candidates {
		if f.Level != "" && !strings.EqualFold(e.Level, f.Level) {
			continue
		}
		if len(f.LevelIn) > 0 {
			ok := false
			for _, lvl := range f.LevelIn {
				if strings.EqualFold(e.Level, lvl) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if !f.After.IsZero() && e.Timestamp.Before(f.After) {
			continue
		}
		if !f.Before.IsZero() && !e.Timestamp.Before(f.Before) {
			continue
		}
		if f.Search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(f.Search)) {
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
