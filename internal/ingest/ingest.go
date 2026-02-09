package ingest

import (
	"bufio"
	"context"
	"io"
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

type TailOptions struct {
	FromStart    bool
	PollInterval time.Duration
}

// TailLogFile streams new log entries as they are appended to a file.
func TailLogFile(ctx context.Context, path string, opts TailOptions) (<-chan types.LogEntry, <-chan error) {
	entries := make(chan types.LogEntry)
	errs := make(chan error, 1)

	go func() {
		defer close(entries)
		defer close(errs)

		f, err := os.Open(path)
		if err != nil {
			errs <- err
			return
		}
		defer f.Close()

		if !opts.FromStart {
			if _, err := f.Seek(0, io.SeekEnd); err != nil {
				errs <- err
				return
			}
		}

		reader := bufio.NewReader(f)
		poll := opts.PollInterval
		if poll <= 0 {
			poll = 500 * time.Millisecond
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(poll)
					continue
				}
				errs <- err
				return
			}

			line = strings.TrimRight(line, "\r\n")
			if strings.TrimSpace(line) == "" {
				continue
			}

			entry, err := parseLine(line)
			if err != nil {
				continue
			}
			entries <- entry
		}
	}()

	return entries, errs
}
