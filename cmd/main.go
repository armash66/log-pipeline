package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/armash/log-pipeline/internal/index"
	"github.com/armash/log-pipeline/internal/ingest"
	"github.com/armash/log-pipeline/internal/store"
	"github.com/armash/log-pipeline/internal/types"
)

func main() {
	file := flag.String("file", "samples/sample.log", "path to log file")
	level := flag.String("level", "", "filter by level (ERROR, WARN, INFO, DEBUG)")
	since := flag.String("since", "", "filter entries newer than duration (e.g. 10m, 1h)")
	search := flag.String("search", "", "filter by substring in message (case-insensitive)")
	jsonOut := flag.Bool("json", false, "output as JSON instead of text")
	limit := flag.Int("limit", 0, "limit output to N entries (0 = no limit)")
	output := flag.String("output", "", "save output to file (e.g. results.json, results.txt)")
	tail := flag.Bool("tail", false, "stream new entries as the file grows")
	tailFromStart := flag.Bool("tail-from-start", false, "when tailing, start from beginning instead of end")
	tailPoll := flag.Duration("tail-poll", 500*time.Millisecond, "when tailing, poll interval (e.g. 250ms, 1s)")
	format := flag.String("format", "plain", "log format: plain, json, logfmt, auto")
	storePath := flag.String("store", "", "append ingested entries to a JSONL store file")
	loadPath := flag.String("load", "", "load entries from a JSONL store file instead of --file")
	useIndex := flag.Bool("index", false, "build in-memory indexes to speed up filtering")
	quiet := flag.Bool("quiet", false, "suppress per-log console output (header still prints)")
	storeHeader := flag.Bool("store-header", false, "also write the run header into the store file before entries")
	flag.Parse()

    if _, err := os.Stat(*file); err != nil {
        if os.IsNotExist(err) {
            log.Fatalf("file not found: %s\nHint: check the path or run with the sample file: --file samples\\sample.log", *file)
        }
        log.Fatalf("failed to access %s: %v", *file, err)
    }

	var cutoff time.Time
	if *since != "" {
		d, err := time.ParseDuration(*since)
		if err != nil {
			log.Fatalf("invalid --since value: %v", err)
		}
		cutoff = time.Now().Add(-d)
	}

	parsedFormat, err := parseFormat(*format)
	if err != nil {
		log.Fatalf("invalid --format: %v", err)
	}

	if *storePath != "" && *loadPath == "" {
		if err := printRunHeader(*file, *storePath); err != nil {
			log.Fatalf("failed to print run header: %v", err)
		}
	}

	if *tail {
		runTail(*file, *level, cutoff, *search, *jsonOut, *limit, *output, *tailFromStart, *tailPoll, parsedFormat, *storePath, *quiet, *storeHeader)
		return
	}

	var entries []types.LogEntry
	if *loadPath != "" {
		entries, err = store.LoadJSONL(*loadPath)
		if err != nil {
			log.Fatalf("failed to load %s: %v", *loadPath, err)
		}
	} else {
		entries, err = ingest.ReadLogFileWithFormat(*file, parsedFormat)
		if err != nil {
			log.Fatalf("failed to read %s: %v", *file, err)
		}
		if *storePath != "" {
			if *storeHeader {
				if err := store.AppendHeader(*storePath, buildRunHeaderText(*file, *storePath)); err != nil {
					log.Fatalf("failed to store header: %v", err)
				}
			}
			if err := store.AppendJSONL(*storePath, entries); err != nil {
				log.Fatalf("failed to store entries: %v", err)
			}
		}
	}

	var filtered []types.LogEntry
	if *useIndex {
		idx := index.Build(entries)
		filtered = index.Filter(entries, idx, *level, cutoff, *search)
	} else {
		filtered = make([]types.LogEntry, 0, len(entries))
		for _, e := range entries {
			if !matchesFilters(e, *level, cutoff, *search) {
				continue
			}
			filtered = append(filtered, e)
		}
	}

	limited := filtered
	if *limit > 0 && len(filtered) > *limit {
		limited = filtered[:*limit]
	}

	var outputText string
	if *jsonOut {
		outputData := map[string]interface{}{
			"total_loaded":  len(entries),
			"after_filters": len(filtered),
			"limited_to":    *limit,
			"entries":       limited,
		}
		data, err := json.MarshalIndent(outputData, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshal JSON: %v", err)
		}
		outputText = string(data)
	} else {
		var textBuilder strings.Builder
		textBuilder.WriteString(fmt.Sprintf("Loaded %d log entries (%d after filters)", len(entries), len(filtered)))
		if *limit > 0 {
			textBuilder.WriteString(fmt.Sprintf(" (showing %d)", len(limited)))
		}
		textBuilder.WriteString("\n")
		for _, e := range limited {
			textBuilder.WriteString(fmt.Sprintf("%s %s %s\n", e.Timestamp.Format(time.RFC3339), e.Level, e.Message))
		}
		outputText = textBuilder.String()
	}

	if *output != "" {
		err := os.WriteFile(*output, []byte(outputText), 0644)
		if err != nil {
			log.Fatalf("failed to write to %s: %v", *output, err)
		}
		fmt.Printf("Output saved to %s\n", *output)
	} else if !*quiet {
		fmt.Print(outputText)
	}
}

func matchesFilters(e types.LogEntry, level string, cutoff time.Time, search string) bool {
	if level != "" && !strings.EqualFold(e.Level, level) {
		return false
	}
	if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
		return false
	}
	if search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(search)) {
		return false
	}
	return true
}

func runTail(path string, level string, cutoff time.Time, search string, jsonOut bool, limit int, output string, fromStart bool, poll time.Duration, format ingest.Format, storePath string, quiet bool, storeHeader bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	entries, errs := ingest.TailLogFile(ctx, path, ingest.TailOptions{
		FromStart:    fromStart,
		PollInterval: poll,
		Format:       format,
	})

	var out *os.File
	if output != "" {
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("failed to open %s: %v", output, err)
		}
		defer f.Close()
		out = f
	}

	write := func(text string) {
		if out != nil {
			if _, err := out.WriteString(text); err != nil {
				log.Fatalf("failed to write to %s: %v", output, err)
			}
			return
		}
		if quiet {
			return
		}
		fmt.Print(text)
	}

	var storeFile *os.File
	if storePath != "" {
		f, err := os.OpenFile(storePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("failed to open %s: %v", storePath, err)
		}
		defer f.Close()
		storeFile = f
		if storeHeader {
			if err := store.AppendHeaderToWriter(storeFile, buildRunHeaderText(path, storePath)); err != nil {
				log.Fatalf("failed to store header: %v", err)
			}
		}
	}

	matched := 0
	for {
		select {
		case err := <-errs:
			if err != nil {
				log.Fatalf("tail error: %v", err)
			}
		case e, ok := <-entries:
			if !ok {
				return
			}
			if storeFile != nil {
				if err := store.AppendJSONLToWriter(storeFile, e); err != nil {
					log.Fatalf("failed to store entry: %v", err)
				}
			}
			if !matchesFilters(e, level, cutoff, search) {
				continue
			}

			if jsonOut {
				data, err := json.Marshal(e)
				if err != nil {
					log.Fatalf("failed to marshal JSON: %v", err)
				}
				write(string(data) + "\n")
			} else {
				write(fmt.Sprintf("%s %s %s\n", e.Timestamp.Format(time.RFC3339), e.Level, e.Message))
			}

			matched++
			if limit > 0 && matched >= limit {
				return
			}
		}
	}
}

func parseFormat(value string) (ingest.Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "plain", "":
		return ingest.FormatPlain, nil
	case "json":
		return ingest.FormatJSON, nil
	case "logfmt":
		return ingest.FormatLogfmt, nil
	case "auto":
		return ingest.FormatAuto, nil
	default:
		return "", fmt.Errorf("expected one of: plain, json, logfmt, auto")
	}
}

func printRunHeader(source string, dest string) error {
	existing, err := countExistingEntries(dest)
	if err != nil {
		return err
	}

	started := time.Now().UTC().Format(time.RFC3339)
	header := buildRunHeaderTextWithValues(started, source, dest, existing)
	fmt.Print("\n\n\n")
	fmt.Print(header)
	fmt.Println()
	return nil
}

func buildRunHeaderText(source string, dest string) string {
	existing, _ := countExistingEntries(dest)
	started := time.Now().UTC().Format(time.RFC3339)
	return buildRunHeaderTextWithValues(started, source, dest, existing)
}

func buildRunHeaderTextWithValues(started string, source string, dest string, existing int) string {
	border := "════════════════════════════════════"
	var b strings.Builder
	b.WriteString(border)
	b.WriteByte('\n')
	b.WriteString("Log ingestion run\n")
	b.WriteString("Started at : ")
	b.WriteString(started)
	b.WriteByte('\n')
	b.WriteString("Source     : ")
	b.WriteString(source)
	b.WriteByte('\n')
	b.WriteString("Destination: ")
	b.WriteString(dest)
	b.WriteByte('\n')
	b.WriteString("Existing   : ")
	b.WriteString(formatCount(existing))
	b.WriteString(" entries\n")
	b.WriteString("Mode       : append (JSONL)\n")
	b.WriteString(border)
	b.WriteByte('\n')
	return b.String()
}

func countExistingEntries(path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func formatCount(n int) string {
	s := strconv.Itoa(n)
	if n < 1000 {
		return s
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre == 0 {
		pre = 3
	}
	b.WriteString(s[:pre])
	for i := pre; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
