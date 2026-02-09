package shard

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/armash/log-pipeline/internal/types"
)

func DayShardPath(baseDir string, t time.Time) string {
	name := t.UTC().Format("2006-01-02") + ".jsonl"
	return filepath.Join(baseDir, name)
}

func GroupByDay(entries []types.LogEntry) map[string][]types.LogEntry {
	out := make(map[string][]types.LogEntry)
	for _, e := range entries {
		key := e.Timestamp.UTC().Format("2006-01-02")
		out[key] = append(out[key], e)
	}
	return out
}

func DaysInRange(after time.Time, before time.Time) []string {
	if after.IsZero() && before.IsZero() {
		return nil
	}

	start := after
	if start.IsZero() {
		start = before
	}
	end := before
	if end.IsZero() {
		end = after
	}
	if end.Before(start) {
		start, end = end, start
	}

	startDay := time.Date(start.UTC().Year(), start.UTC().Month(), start.UTC().Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(end.UTC().Year(), end.UTC().Month(), end.UTC().Day(), 0, 0, 0, 0, time.UTC)

	days := make([]string, 0)
	for d := startDay; !d.After(endDay); d = d.Add(24 * time.Hour) {
		days = append(days, d.Format("2006-01-02"))
	}
	return days
}

func SortEntries(entries []types.LogEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})
}

func ShardPathsForRange(baseDir string, after time.Time, before time.Time) []string {
	days := DaysInRange(after, before)
	if len(days) == 0 {
		return nil
	}
	paths := make([]string, 0, len(days))
	for _, day := range days {
		paths = append(paths, filepath.Join(baseDir, fmt.Sprintf("%s.jsonl", day)))
	}
	return paths
}

func AllShardPaths(baseDir string) ([]string, error) {
	pattern := filepath.Join(baseDir, "*.jsonl")
	return filepath.Glob(pattern)
}

func ParseShardDate(path string) (time.Time, bool) {
	base := filepath.Base(path)
	if !strings.HasSuffix(base, ".jsonl") {
		return time.Time{}, false
	}
	day := strings.TrimSuffix(base, ".jsonl")
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
