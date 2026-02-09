package ingest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/armash/log-pipeline/internal/types"
)

type Format string

const (
	FormatAuto   Format = "auto"
	FormatPlain  Format = "plain"
	FormatJSON   Format = "json"
	FormatLogfmt Format = "logfmt"
)

// ReadLogFile reads a log file line-by-line and returns parsed LogEntry slices.
func ReadLogFile(path string) ([]types.LogEntry, error) {
	return ReadLogFileWithFormat(path, FormatPlain)
}

// ReadLogFileWithFormat reads a log file using a specific format or auto-detects.
func ReadLogFileWithFormat(path string, format Format) ([]types.LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ReadLogReaderWithFormat(f, format)
}

// ReadLogReaderWithFormat reads log lines from a reader using a specific format or auto-detects.
func ReadLogReaderWithFormat(r io.Reader, format Format) ([]types.LogEntry, error) {
	scanner := bufio.NewScanner(r)
	entries := make([]types.LogEntry, 0)
	detected := format
	seenFirstLine := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !seenFirstLine {
			seenFirstLine = true
			if format == FormatAuto {
				detected = detectFormat(line)
			}
		}

		entry, err := parseLineWithFormat(line, detected)
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

func parseLineWithFormat(line string, format Format) (types.LogEntry, error) {
	switch format {
	case FormatJSON:
		return parseJSONLine(line)
	case FormatLogfmt:
		return parseLogfmtLine(line)
	case FormatPlain:
		return parseLine(line)
	case FormatAuto:
		return parseLineWithFormat(line, detectFormat(line))
	default:
		return types.LogEntry{}, errors.New("unknown format")
	}
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

func detectFormat(line string) Format {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return FormatJSON
	}
	if strings.Contains(trimmed, "=") {
		return FormatLogfmt
	}
	return FormatPlain
}

func parseJSONLine(line string) (types.LogEntry, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return types.LogEntry{}, err
	}

	tsRaw := firstStringFromMap(raw, "timestamp", "time", "ts")
	level := firstStringFromMap(raw, "level", "severity")
	message := firstStringFromMap(raw, "message", "msg")

	if tsRaw == "" || level == "" || message == "" {
		return types.LogEntry{}, os.ErrInvalid
	}

	t, err := time.Parse(time.RFC3339, tsRaw)
	if err != nil {
		return types.LogEntry{}, err
	}

	return types.LogEntry{
		Timestamp: t,
		Level:     level,
		Message:   message,
	}, nil
}

func parseLogfmtLine(line string) (types.LogEntry, error) {
	fields := parseLogfmtFields(line)
	if len(fields) == 0 {
		return types.LogEntry{}, os.ErrInvalid
	}

	tsRaw := firstStringFromStringMap(fields, "timestamp", "time", "ts")
	level := firstStringFromStringMap(fields, "level", "severity")
	message := firstStringFromStringMap(fields, "message", "msg")

	if tsRaw == "" || level == "" || message == "" {
		return types.LogEntry{}, os.ErrInvalid
	}

	t, err := time.Parse(time.RFC3339, tsRaw)
	if err != nil {
		return types.LogEntry{}, err
	}

	return types.LogEntry{
		Timestamp: t,
		Level:     level,
		Message:   message,
	}, nil
}

func firstStringFromStringMap(m map[string]string, keys ...string) string {
	for _, key := range keys {
		if val, ok := m[key]; ok && val != "" {
			return val
		}
	}
	return ""
}

func firstStringFromMap(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func parseLogfmtFields(line string) map[string]string {
	result := make(map[string]string)
	i := 0
	n := len(line)
	for i < n {
		for i < n && line[i] == ' ' {
			i++
		}
		if i >= n {
			break
		}
		startKey := i
		for i < n && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i >= n || line[i] != '=' {
			for i < n && line[i] != ' ' {
				i++
			}
			continue
		}
		key := line[startKey:i]
		i++
		if i >= n {
			result[key] = ""
			break
		}

		var val string
		if line[i] == '"' {
			i++
			startVal := i
			for i < n && line[i] != '"' {
				i++
			}
			val = line[startVal:i]
			if i < n && line[i] == '"' {
				i++
			}
		} else {
			startVal := i
			for i < n && line[i] != ' ' {
				i++
			}
			val = line[startVal:i]
		}

		if key != "" {
			result[key] = val
		}
	}
	return result
}

type TailOptions struct {
	FromStart    bool
	PollInterval time.Duration
	Format       Format
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
		detected := opts.Format
		seenFirstLine := false
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

			if !seenFirstLine {
				seenFirstLine = true
				if opts.Format == FormatAuto || opts.Format == "" {
					detected = detectFormat(line)
				}
			}

			entry, err := parseLineWithFormat(line, detected)
			if err != nil {
				continue
			}
			entries <- entry
		}
	}()

	return entries, errs
}
