package types

import "time"

// LogEntry represents a single log line entry
type LogEntry struct {
	Timestamp time.Time
	Level     string // ERROR, WARN, INFO, DEBUG
	Message   string
}
