package main

import (
    "encoding/json"
    "flag"
	"fmt"
	"log"
    "os"
    "strings"
    "time"

    "github.com/armash/log-pipeline/internal/ingest"
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
    flag.Parse()

    if _, err := os.Stat(*file); err != nil {
        if os.IsNotExist(err) {
            log.Fatalf("file not found: %s\nHint: check the path or run with the sample file: --file samples\\sample.log", *file)
        }
        log.Fatalf("failed to access %s: %v", *file, err)
    }

	entries, err := ingest.ReadLogFile(*file)
	if err != nil {
		log.Fatalf("failed to read %s: %v", *file, err)
	}

	var cutoff time.Time
	if *since != "" {
		d, err := time.ParseDuration(*since)
		if err != nil {
			log.Fatalf("invalid --since value: %v", err)
		}
		cutoff = time.Now().Add(-d)
	}

	filtered := make([]types.LogEntry, 0, len(entries))
	for _, e := range entries {
		if *level != "" && !strings.EqualFold(e.Level, *level) {
			continue
		}
		if !cutoff.IsZero() && e.Timestamp.Before(cutoff) {
			continue
		}
		if *search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(*search)) {
			continue
		}
		filtered = append(filtered, e)
	}

	limited := filtered
	if *limit > 0 && len(filtered) > *limit {
		limited = filtered[:*limit]
	}

	var outputText string
	if *jsonOut {
		outputData := map[string]interface{}{
			"total_loaded":   len(entries),
			"after_filters":  len(filtered),
			"limited_to":     *limit,
			"entries":        limited,
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
	} else {
		fmt.Print(outputText)
	}
}
