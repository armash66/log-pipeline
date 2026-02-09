package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/armash/log-pipeline/internal/types"
)

// AppendJSONL appends entries as JSON lines to a file.
func AppendJSONL(path string, entries []types.LogEntry) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, e := range entries {
		if err := AppendJSONLToWriter(f, e); err != nil {
			return err
		}
	}
	return nil
}

// AppendJSONLToWriter writes a single entry as JSON line to a writer.
func AppendJSONLToWriter(f *os.File, entry types.LogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

// LoadJSONL reads entries from a JSONL file.
func LoadJSONL(path string) ([]types.LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	entries := make([]types.LogEntry, 0)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e types.LogEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// WriteSnapshot writes all entries to a JSON file (pretty-printed).
func WriteSnapshot(path string, entries []types.LogEntry) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// AppendHeader writes a header block to the store file.
func AppendHeader(path string, header string) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return AppendHeaderToWriter(f, header)
}

// AppendHeaderToWriter writes a header block to a writer.
func AppendHeaderToWriter(f *os.File, header string) error {
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	return nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0755)
}
