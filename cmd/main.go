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

	if *jsonOut {
		output := map[string]interface{}{
			"total_loaded":   len(entries),
			"after_filters":  len(filtered),
			"entries":        filtered,
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshal JSON: %v", err)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("Loaded %d log entries (%d after filters)\n", len(entries), len(filtered))
		for i, e := range filtered {
			if i >= 20 {
				break
			}
			fmt.Printf("%s %s %s\n", e.Timestamp.Format(time.RFC3339), e.Level, e.Message)
		}
	}
}
