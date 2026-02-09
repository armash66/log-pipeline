# Log Pipeline

A Go-based log ingestion and query system with a CLI, HTTP API, and Web UI.

---

## What It Does

- Ingests log files into structured entries (`timestamp`, `level`, `message`)
- Filters by level, time range, substring, and DSL query
- Streams new log lines (`--tail`)
- Persists to JSONL (append-only)
- In-memory indexing for faster queries
- Snapshots with metadata + fast reload
- HTTP API for querying and ingestion
- Web UI for browsing and uploading logs
- Time-based sharding and retention cleanup

---

## Project Structure

```
log-pipeline/
├── cmd/                    # CLI entrypoint
│   └── main.go
├── internal/
│   ├── config/             # JSON config loader
│   ├── engine/             # shared load/query logic (CLI + HTTP)
│   ├── index/              # in-memory indexing + snapshot index
│   ├── ingest/             # parsers + tailing
│   ├── query/              # DSL parsing + filter merge
│   ├── server/             # HTTP API
│   ├── shard/              # daily shard helpers
│   ├── snapshot/           # snapshot writer/reader
│   ├── store/              # JSONL persistence
│   └── types/              # LogEntry model
├── samples/                # example logs
├── web/                    # UI (served at /ui)
├── go.mod
└── README.md
```

---

## Quick Start

```powershell
go run ./cmd/main.go
```

---

## CLI Usage

### Common flags

- `--file` path to log file (default `samples/sample.log`)
- `--format` `plain|json|logfmt|auto`
- `--level` filter by level
- `--since` duration (`10m`, `2h30m`, `1d`, `1w2d`)
- `--search` substring in message
- `--query` DSL (`level=ERROR OR level=WARN`, `level in (ERROR,WARN) message~"auth"`)
- `--limit` max output entries
- `--json` output as JSON
- `--output` save output to a file
- `--tail` stream new entries
- `--tail-from-start` tail from beginning
- `--tail-poll` polling interval

### Persistence + indexing

- `--store` append to JSONL file
- `--load` load from JSONL file
- `--store-header` write run header into store
- `--quiet` suppress per-log output
- `--index` build index for faster filtering
- `--replay` load existing store into memory before ingest
- `--snapshot` create snapshot file
- `--snapshot-load` load from snapshot file
- `--retention` drop entries older than duration

### Metrics + service

- `--metrics` print metrics
- `--metrics-file` write metrics to file
- `--serve` run HTTP API
- `--port` server port (default 8080)
- `--api-key` require `X-API-Key` for HTTP ingest

### Sharding + cleanup

- `--shard-dir` write daily shards to directory
- `--shard-read` read from shards instead of file
- `--cleanup` clean old shards (requires retention)
- `--cleanup-dry-run` show cleanup plan only
- `--cleanup-confirm` confirm deletion

### Config

- `--config` load settings from JSON config

---

## CLI Examples

Basic run:
```powershell
go run ./cmd/main.go --file samples/app.log
```

Filter:
```powershell
go run ./cmd/main.go --file samples/app.log --level ERROR
go run ./cmd/main.go --file samples/app.log --since 10m --search "timeout"
```

DSL:
```powershell
go run ./cmd/main.go --file samples/app.log --query "level=ERROR OR level=WARN"
go run ./cmd/main.go --file samples/app.log --query "level in (ERROR,WARN) message~\"auth\""
```

Store + load:
```powershell
go run ./cmd/main.go --file samples/app.log --store data/store.jsonl
go run ./cmd/main.go --load data/store.jsonl --level ERROR --index
```

Snapshot:
```powershell
go run ./cmd/main.go --file samples/app.log --snapshot data/snapshot.json
go run ./cmd/main.go --snapshot-load data/snapshot.json --query "level=ERROR"
```

Shards:
```powershell
go run ./cmd/main.go --file samples/app.log --shard-dir data/shards
go run ./cmd/main.go --shard-dir data/shards --shard-read --query "after=2026-02-08T00:00:00Z before=2026-02-09T00:00:00Z"
```

Cleanup:
```powershell
go run ./cmd/main.go --shard-dir data/shards --retention 7d --cleanup --cleanup-dry-run
go run ./cmd/main.go --shard-dir data/shards --retention 7d --cleanup --cleanup-confirm
```

---

## HTTP API

Start server:
```powershell
go run ./cmd/main.go --file samples/app.log --serve --port 8080 --store data/store.jsonl
```

Endpoints:
```powershell
curl http://localhost:8080/health
curl "http://localhost:8080/query?level=ERROR&since=10m&search=auth&limit=5"
curl http://localhost:8080/metrics
```

HTTP ingest:
```powershell
curl.exe -X POST "http://localhost:8080/ingest" -H "Content-Type: application/json" -d "{\"entry\":{\"timestamp\":\"2026-02-09T17:10:12Z\",\"level\":\"INFO\",\"message\":\"hello\"}}"
```

File upload:
```powershell
curl.exe -X POST "http://localhost:8080/ingest/file" -H "Content-Type: application/json" --data-binary "@body.json"
```

---

## Web UI

Open:
```
http://localhost:8080/ui/
```

Features:
- Filters + DSL query
- Upload and ingest logs
- Metrics panel
- Help page

---

## Config File (example)

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

Run:
```powershell
go run ./cmd/main.go --config config.json
```

---
