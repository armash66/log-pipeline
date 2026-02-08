package ingest

import (
	"testing"
	"time"
	"github.com/armash/log-pipeline/internal/types"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantErr bool
		want    types.LogEntry
	}{
		{
			name:    "valid entry",
			line:    "2026-02-08T10:15:32Z ERROR Database connection failed",
			wantErr: false,
			want: types.LogEntry{
				Timestamp: time.Date(2026, 2, 8, 10, 15, 32, 0, time.UTC),
				Level:     "ERROR",
				Message:   "Database connection failed",
			},
		},
		{
			name:    "valid entry with multiple words in message",
			line:    "2026-02-08T16:00:10Z DEBUG Initializing database connection pool",
			wantErr: false,
			want: types.LogEntry{
				Timestamp: time.Date(2026, 2, 8, 16, 0, 10, 0, time.UTC),
				Level:     "DEBUG",
				Message:   "Initializing database connection pool",
			},
		},
		{
			name:    "missing message",
			line:    "2026-02-08T10:15:32Z ERROR",
			wantErr: true,
		},
		{
			name:    "missing level",
			line:    "2026-02-08T10:15:32Z",
			wantErr: true,
		},
		{
			name:    "invalid timestamp",
			line:    "not-a-timestamp ERROR Some message",
			wantErr: true,
		},
		{
			name:    "single word message",
			line:    "2026-02-08T10:15:32Z WARN Alert",
			wantErr: false,
			want: types.LogEntry{
				Timestamp: time.Date(2026, 2, 8, 10, 15, 32, 0, time.UTC),
				Level:     "WARN",
				Message:   "Alert",
			},
		},
		{
			name:    "uppercase and lowercase mix",
			line:    "2026-02-08T10:15:32Z INFO User Login Successful",
			wantErr: false,
			want: types.LogEntry{
				Timestamp: time.Date(2026, 2, 8, 10, 15, 32, 0, time.UTC),
				Level:     "INFO",
				Message:   "User Login Successful",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if got.Timestamp != tt.want.Timestamp {
					t.Errorf("parseLine() timestamp = %v, want %v", got.Timestamp, tt.want.Timestamp)
				}
				if got.Level != tt.want.Level {
					t.Errorf("parseLine() level = %v, want %v", got.Level, tt.want.Level)
				}
				if got.Message != tt.want.Message {
					t.Errorf("parseLine() message = %v, want %v", got.Message, tt.want.Message)
				}
			}
		})
	}
}

func TestReadLogFile(t *testing.T) {
	tests := []struct {
		name      string
		filePath  string
		wantErr   bool
		wantCount int
	}{
		{
			name:      "valid sample log",
			filePath:  "../../samples/sample.log",
			wantErr:   false,
			wantCount: 10,
		},
		{
			name:      "valid app log",
			filePath:  "../../samples/app.log",
			wantErr:   false,
			wantCount: 25,
		},
		{
			name:      "non-existent file",
			filePath:  "nonexistent.log",
			wantErr:   true,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadLogFile(tt.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadLogFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("ReadLogFile() got %d entries, want %d", len(got), tt.wantCount)
			}
		})
	}
}
