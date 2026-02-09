package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/armash/log-pipeline/internal/index"
	"github.com/armash/log-pipeline/internal/types"
)

const Version = 1

type Metadata struct {
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"createdAt"`
	EntryCount  int       `json:"entryCount"`
	SourceFiles []string  `json:"sourceFiles"`
}

type Snapshot struct {
	Metadata Metadata          `json:"metadata"`
	Entries  []types.LogEntry  `json:"entries"`
	Index    index.SnapshotIndex `json:"index"`
}

func Create(path string, entries []types.LogEntry, sources []string) error {
	if err := ensureDir(path); err != nil {
		return err
	}

	idx := index.Build(entries)
	snap := Snapshot{
		Metadata: Metadata{
			Version:     Version,
			CreatedAt:   time.Now().UTC(),
			EntryCount:  len(entries),
			SourceFiles: sources,
		},
		Entries: entries,
		Index:   index.ToSnapshotIndex(idx, entries),
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path)
	}
	return os.Rename(tmp, path)
}

func Load(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}
