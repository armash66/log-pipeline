package main

import (
	"fmt"
	"log"

	"github.com/armash/log-pipeline/internal/ingest"
)

func main() {
	fmt.Println("Log Pipeline - Ingestion demo")

	entries, err := ingest.ReadLogFile("samples/sample.log")
	if err != nil {
		log.Fatalf("failed to read sample.log: %v", err)
	}

	fmt.Printf("Loaded %d log entries\n", len(entries))
	// print first few entries
	for i, e := range entries {
		if i >= 5 {
			break
		}
		fmt.Printf("%s %s %s\n", e.Timestamp.Format("2006-01-02T15:04:05Z"), e.Level, e.Message)
	}
}
