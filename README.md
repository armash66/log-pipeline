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
