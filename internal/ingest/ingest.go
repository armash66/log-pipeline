package ingest

import (
	"bufio"
	"os"
	"strings"
	"time"
	"github.com/armash/log-pipeline/internal/types"
)

// ReadLogFile reads a log file line-by-line and returns parsed LogEntry slices.
func ReadLogFile(path string) ([]types.LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	entries := make([]types.LogEntry, 0)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		entry, err := parseLine(line)
		if err != nil {
			// skip malformed lines
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func parseLine(line string) (types.LogEntry, error) {
	// Expected format: <timestamp> <LEVEL> <message...>
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return types.LogEntry{}, os.ErrInvalid
	}
	ts := parts[0]
	level := parts[1]
	message := strings.Join(parts[2:], " ")

	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return types.LogEntry{}, err
	}

	return types.LogEntry{
		Timestamp: t,
		Level:     level,
		Message:   message,
	}, nil
}
