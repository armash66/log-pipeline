# Log Pipeline

A lightweight log aggregator + query engine in Go, now with CLI, HTTP API, and a Web UI.

**Why this is cool**
- Starts as a minimal log reader, grows into a real log service.
- Clean, incremental phases from ingestion → querying → indexing → persistence → API → UI.
- Built for learning and shipping, not just theory.

---

**What It Does**

Core capabilities:
- Ingests logs into structured entries (`timestamp`, `level`, `message`)
- Filters by level, time range, substring, and DSL
- Streams with `--tail`
- Persists to JSONL (append-only)
- Indexes for faster queries
- Snapshots with metadata + replay
- HTTP API for query + ingest
- Web UI for browsing and querying
- Time-based sharding by day
- Cleanup for retention

---

**Project Structure**

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

**Quick Start**

```powershell
go run ./cmd/main.go
```

---

**CLI Flags (full list)**

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
- `--store` append to JSONL file
- `--load` load from JSONL file
- `--store-header` write run header into store
- `--quiet` suppress per-log output
- `--index` build index for faster filtering
- `--replay` load existing store into memory before ingest
- `--snapshot` create snapshot file
- `--snapshot-load` load from snapshot file
- `--retention` drop entries older than duration
- `--metrics` print metrics
- `--metrics-file` write metrics to file
- `--serve` run HTTP API
- `--port` server port (default 8080)
- `--api-key` require `X-API-Key` for HTTP ingest
- `--shard-dir` write daily shards to directory
- `--shard-read` read from shards instead of file
- `--cleanup` clean old shards (requires retention)
- `--cleanup-dry-run` show cleanup plan only
- `--cleanup-confirm` confirm deletion
- `--config` load settings from JSON config

---

**Usage Examples**

Run:
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

**HTTP API**

Start:
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

Upload file (multipart):
```powershell
curl.exe -X POST "http://localhost:8080/ingest/file" -H "Content-Type: application/json" --data-binary "@body.json"
```

---

**Web UI**

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

**Config File (example)**

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

**Development Phases (From Zero → Full System)**

Phase 0: Foundation
- Go module, structure, LogEntry model, sample log, README

Phase 1: Ingestion
- Read file line-by-line, parse into entries

Phase 2: Query Engine
- Filters by level, time, substring + CLI flags

Phase 3: Streaming
- Tail mode with polling

Phase 4: Indexing
- In-memory indexes by level and time buckets

Phase 5: Query Planning
- `--explain` plan output

Phase 6: Persistence
- JSONL store + replay + snapshot

Phase 7: Config + Extensibility
- JSON config file, retention window

Phase 8: Metrics + Observability
- Run metrics output

Phase 9: HTTP Service
- `/health`, `/query`, `/metrics`, `/ingest`

Phase 10: Snapshot & Replay
- Metadata + atomic snapshot + fast load

Phase 11: Sharding
- Daily JSONL shards + range loading

Phase 12: HTTP Ingestion
- POST ingest single/batch + API key

Phase 13: DSL v2
- OR + `level in (...)`

Phase 14: Cleanup
- Retention cleanup with dry-run and confirm

Phase 15: Web UI
- Query dashboard + upload + help

---

**Suggested Commit**

```bash
git add README.md
git commit -m "docs: add full project README"
```
