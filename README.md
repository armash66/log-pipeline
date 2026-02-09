# Log Pipeline

A lightweight log aggregator and query engine built in Go.

## What

Log Pipeline reads log files, parses them into structured entries, and provides tools to query and analyze those entries.

## Why

- Learn Go fundamentals: file I/O, data structures, concurrency
- Prototype real systems: basic ideas from ELK/Fluentd-style pipelines
- Build incrementally: start simple and add features as needed

## Status

Active development — the repository contains an initial foundation (module, folder layout, `LogEntry` model, and a sample log). The project will grow iteratively; timeline details are intentionally omitted so development can flow naturally.

### Project Structure

```
log-pipeline/
├── cmd/               # Entry point
│   └── main.go
├── internal/          # Internal packages
│   └── types/         # Data models
│       └── types.go
├── samples/           # Sample log files
│   └── sample.log
├── go.mod             # Module definition
└── README.md          # This file
```

## Quick Start

```bash
go run ./cmd/main.go
```

## Next Steps

- Implement ingestion: read lines, parse timestamp/level/message, store entries in-memory
- Add a simple query CLI to filter by level and time range

---

Built as an iterative learning project.
 
## Current Implementation

This repository now contains a working CLI log pipeline with ingestion, filtering, and basic output features. Key implemented items:

- Ingestion: `internal/ingest.ReadLogFile` reads log files line-by-line and parses entries as `LogEntry` (timestamp, level, message).
- CLI flags (in `cmd/main.go`):
	- `--file` (path to log file, default `samples/sample.log`)
	- `--level` (ERROR, WARN, INFO, DEBUG)
	- `--since` (duration like `10m`, `1h` to filter recent entries)
	- `--search` (case-insensitive substring search in message)
	- `--json` (output as pretty JSON)
	- `--limit` (limit number of entries in output)
	- `--output` (save output to a file; writes text or JSON depending on `--json`)
	- `--format` (plain, json, logfmt, auto)
	- `--tail` (stream new entries as file grows)
	- `--tail-from-start` (tail from beginning instead of end)
	- `--tail-poll` (poll interval when tailing)
	- `--store` (append ingested entries to a JSONL store file)
	- `--load` (load entries from a JSONL store file instead of `--file`)
	- `--index` (build in-memory indexes to speed up filtering)
	- `--quiet` (suppress per-log console output; header still prints)
	- `--store-header` (also write the run header into the store file)
	- `--query` (simple query DSL: `level=ERROR message~"auth" since=10m after=... before=...`)
	- `--explain` (print query plan before executing)
	- `--replay` (load existing store entries into memory before ingesting new ones)
	- `--snapshot` (write a snapshot file with entries, indexes, and metadata)
	- `--snapshot-load` (load entries from a snapshot instead of parsing logs)
	- `--retention` (drop entries older than duration, e.g. `24h`, `7d`)
	- `--config` (load settings from a JSON config file)
	- `--metrics` (print ingestion/query metrics)
	- `--metrics-file` (write metrics to a file)
	- `--serve` (run HTTP server mode)
	- `--port` (server port for `--serve`, default 8080)
	- `--shard-dir` (write daily JSONL shards to this directory)
	- `--shard-read` (read entries from shards in `--shard-dir` instead of `--file`)
	- `--api-key` (require `X-API-Key` for HTTP ingest)
- Unit tests: `internal/ingest/ingest_test.go` covers `parseLine` and `ReadLogFile` behaviors.
- Sample logs: `samples/sample.log` and `samples/app.log` are included for testing and demos.

## Usage Examples

Run the demo with defaults:

```powershell
go run ./cmd/main.go
```

Filter and print errors:

```powershell
go run ./cmd/main.go --file samples/app.log --level ERROR
```

Search + JSON + limit + write to file:

```powershell
go run ./cmd/main.go --file samples/app.log --search "database" --json --limit 2 --output results.json
```

Save text output:

```powershell
go run ./cmd/main.go --file samples/app.log --level ERROR --output errors.txt
```

Parse JSON logs:

```powershell
go run ./cmd/main.go --file samples/json.log --format json
```

Parse logfmt logs:

```powershell
go run ./cmd/main.go --file samples/logfmt.log --format logfmt
```

Auto-detect format:

```powershell
go run ./cmd/main.go --file samples/json.log --format auto
```

Store entries to a JSONL file and query from it:

```powershell
go run ./cmd/main.go --file samples/app.log --store data/store.jsonl
go run ./cmd/main.go --load data/store.jsonl --level ERROR --index
```

Replay + snapshot:

```powershell
go run ./cmd/main.go --file samples/app.log --store data/store.jsonl --replay --snapshot data/snapshot.json
go run ./cmd/main.go --snapshot-load data/snapshot.json --query "level=ERROR"
```

Config file example (`config.json`):

```json
{
  "file": "samples/app.log",
  "format": "plain",
  "store": "data/store.jsonl",
  "replay": true,
  "index": true,
  "query": "level=ERROR message~timeout",
  "retention": "72h"
}
```

Run with config:

```powershell
go run ./cmd/main.go --config config.json
```

Metrics:

```powershell
go run ./cmd/main.go --file samples/app.log --metrics
go run ./cmd/main.go --file samples/app.log --metrics --metrics-file data/metrics.txt
```

Server mode:

```powershell
go run ./cmd/main.go --file samples/app.log --serve --port 8080
```

Endpoints:

```powershell
curl http://localhost:8080/health
curl "http://localhost:8080/query?level=ERROR&since=10m&search=auth&limit=5"
curl http://localhost:8080/metrics
curl -X POST http://localhost:8080/ingest -H "Content-Type: application/json" -d "{\"entry\":{\"timestamp\":\"2026-02-09T17:10:12Z\",\"level\":\"INFO\",\"message\":\"hello\"}}"
```

Sharding (by day):

```powershell
go run ./cmd/main.go --file samples/app.log --shard-dir data/shards
go run ./cmd/main.go --shard-dir data/shards --shard-read --query "after=2026-02-08T00:00:00Z before=2026-02-09T00:00:00Z"
```

Query DSL examples:

```powershell
go run ./cmd/main.go --file samples/app.log --query "level=ERROR message~timeout"
go run ./cmd/main.go --file samples/app.log --query "since=10m message~\"auth\""
go run ./cmd/main.go --file samples/app.log --query "after=2026-02-08T16:00:00Z before=2026-02-08T17:00:00Z"
```

Explain plan:

```powershell
go run ./cmd/main.go --file samples/app.log --query "level=ERROR message~timeout" --index --explain
```

Run parser tests:

```powershell
go test ./internal/ingest -v
```

## Next Work (optional)

- Streaming/tail mode (`--tail`) to follow files in real time
- More filters (substring DSL), indexing for speed, persistence, HTTP API

## Suggested Commit

```bash
git add .
git commit -m "feat: add ingestion, CLI filters (--file,--level,--since,--search,--json,--limit,--output), tests and samples"
```

If you'd like, I can add streaming/tail next or prepare an integration test harness.

**Development Process**

- **Ingestion implementation:** `internal/ingest.ReadLogFile` reads `samples/*.log` line-by-line, expects an RFC3339 timestamp as the first token, a log level as the second token (e.g. `ERROR`, `WARN`, `INFO`, `DEBUG`), and the rest as the message. Malformed lines are skipped.
- **Key files:**
	- [cmd/main.go](cmd/main.go) — demo runner that loads a log and prints a summary
	- [internal/ingest/ingest.go](internal/ingest/ingest.go) — ingestion + parser
	- [internal/types/types.go](internal/types/types.go) — `LogEntry` model
	- [samples/sample.log](samples/sample.log) — example input
- **How to run locally:**

```powershell
go run ./cmd/main.go
```

- **When adding support for other formats:**
	1. Add a new parser (e.g. JSON/syslog) under `internal/ingest`.
	2. Make parsers pluggable or auto-detect by inspecting the first line.
	3. Add unit tests for parsing edge cases.

- **Suggested git commit** for the current changes:

```bash
git add .
git commit -m "feat: add ingestion (ReadLogFile), sample log, and demo runner"
```

This README section captures the current implementation and quick next steps for extending the parser or building a CLI.
