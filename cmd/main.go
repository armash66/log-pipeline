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

	"github.com/armash/log-pipeline/internal/config"
	"github.com/armash/log-pipeline/internal/engine"
	"github.com/armash/log-pipeline/internal/ingest"
	"github.com/armash/log-pipeline/internal/query"
	"github.com/armash/log-pipeline/internal/server"
	"github.com/armash/log-pipeline/internal/shard"
	"github.com/armash/log-pipeline/internal/snapshot"
	"github.com/armash/log-pipeline/internal/store"
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
	queryStr := flag.String("query", "", "query DSL (e.g. level=ERROR message~\"auth\" since=10m)")
	explain := flag.Bool("explain", false, "print query plan before executing")
	replay := flag.Bool("replay", false, "load existing store entries into memory before ingesting new ones")
	snapshotPath := flag.String("snapshot", "", "write a full snapshot of entries to a JSON file")
	snapshotLoad := flag.String("snapshot-load", "", "load entries from a snapshot file instead of parsing logs")
	retention := flag.String("retention", "", "drop entries older than duration (e.g. 24h, 7d)")
	configPath := flag.String("config", "", "load settings from a JSON config file")
	metricsFlag := flag.Bool("metrics", false, "print ingestion/query metrics")
	metricsFile := flag.String("metrics-file", "", "write metrics to a file (text)")
	serve := flag.Bool("serve", false, "run HTTP server mode")
	port := flag.Int("port", 8080, "server port for --serve")
	shardDir := flag.String("shard-dir", "", "write daily JSONL shards to this directory")
	shardRead := flag.Bool("shard-read", false, "read entries from shards in --shard-dir instead of --file")
	apiKey := flag.String("api-key", "", "API key required for POST /ingest")
	flag.Parse()

	runStart := time.Now()
	setFlags := make(map[string]bool)
	flag.CommandLine.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	if *configPath != "" {
		cfg, err := config.Load(*configPath)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		applyConfig(cfg, setFlags, file, level, since, search, jsonOut, limit, output, tail, tailFromStart, tailPoll, format, storePath, loadPath, useIndex, quiet, storeHeader, queryStr, explain, replay, snapshotPath, snapshotLoad, retention, metricsFlag, metricsFile, serve, port, shardDir, shardRead, apiKey)
	}

	if *shardRead && *shardDir == "" {
		log.Fatalf("--shard-read requires --shard-dir")
	}

	if *loadPath == "" && *snapshotLoad == "" && !*shardRead {
		if _, err := os.Stat(*file); err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("file not found: %s\nHint: check the path or run with the sample file: --file samples\\sample.log", *file)
			}
			log.Fatalf("failed to access %s: %v", *file, err)
		}
	}

	var cutoff time.Time
	if *since != "" {
		d, err := time.ParseDuration(*since)
		if err != nil {
			log.Fatalf("invalid --since value: %v", err)
		}
		cutoff = time.Now().Add(-d)
	}

	var retentionDur time.Duration
	if *retention != "" {
		d, err := time.ParseDuration(*retention)
		if err != nil {
			log.Fatalf("invalid --retention value: %v", err)
		}
		retentionDur = d
	}

	parsedFormat, err := parseFormat(*format)
	if err != nil {
		log.Fatalf("invalid --format: %v", err)
	}

	filters := query.BuildFilters(*level, cutoff, *search)
	if *queryStr != "" {
		qf, err := query.Parse(*queryStr)
		if err != nil {
			log.Fatalf("invalid --query: %v", err)
		}
		merged, err := query.MergeFilters(filters, qf)
		if err != nil {
			log.Fatalf("invalid --query: %v", err)
		}
		filters = merged
	}

	var shardPaths []string
	if *shardRead {
		if !filters.After.IsZero() || !filters.Before.IsZero() {
			shardPaths = shard.ShardPathsForRange(*shardDir, filters.After, filters.Before)
		} else {
			paths, err := shard.AllShardPaths(*shardDir)
			if err != nil {
				log.Fatalf("failed to list shards: %v", err)
			}
			shardPaths = paths
		}
	}

	if *serve {
		result, err := engine.LoadEntries(engine.LoadOptions{
			File:         *file,
			Format:       parsedFormat,
			LoadPath:     *loadPath,
			SnapshotPath: *snapshotLoad,
			StorePath:    "",
			ShardDir:     *shardDir,
			ShardPaths:   shardPaths,
			Replay:       *replay,
			Retention:    retentionDur,
		})
		if err != nil {
			log.Fatalf("failed to load entries: %v", err)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		srv := server.New(result.Entries, result.Stats, *useIndex, result.Index, *storePath, *shardDir, *apiKey)
		addr := fmt.Sprintf(":%d", *port)
		if err := srv.Start(ctx, addr); err != nil {
			log.Fatalf("server error: %v", err)
		}
		return
	}

	if *storePath != "" && *loadPath == "" {
		if err := printRunHeader(*file, *storePath); err != nil {
			log.Fatalf("failed to print run header: %v", err)
		}
	}

	if *tail {
		if *explain {
			printPlan(buildQueryPlan(query.BuildFilters(*level, cutoff, *search), *queryStr, *useIndex))
		}
		runTail(*file, *level, cutoff, *search, *jsonOut, *limit, *output, *tailFromStart, *tailPoll, parsedFormat, *storePath, *quiet, *storeHeader)
		return
	}

	result, err := engine.LoadEntries(engine.LoadOptions{
		File:            *file,
		Format:          parsedFormat,
		LoadPath:        *loadPath,
		SnapshotPath:    *snapshotLoad,
		StorePath:       *storePath,
		ShardDir:        *shardDir,
		ShardPaths:      shardPaths,
		Replay:          *replay,
		Retention:       retentionDur,
		StoreHeaderText: headerText(*storePath, *storeHeader, *file),
	})
	if err != nil {
		log.Fatalf("failed to load entries: %v", err)
	}

	entries := result.Entries
	loadStats := result.Stats

	if *snapshotPath != "" {
		if err := snapshot.Create(*snapshotPath, entries, snapshotSources(*file, *loadPath, *snapshotLoad)); err != nil {
			log.Fatalf("failed to write snapshot: %v", err)
		}
	}

	if *explain {
		printPlan(buildQueryPlan(filters, *queryStr, *useIndex))
	}

	filtered, metricsResult := engine.QueryEntries(entries, loadStats, engine.QueryOptions{
		Filters:  filters,
		UseIndex: *useIndex,
		Limit:    *limit,
		Index:    result.Index,
	})

	limited := filtered
	afterFilters := len(entries) - metricsResult.LogsFilteredOut

	var outputText string
	if *jsonOut {
		outputData := map[string]interface{}{
			"total_loaded":  len(entries),
			"after_filters": afterFilters,
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
		textBuilder.WriteString(fmt.Sprintf("Loaded %d log entries (%d after filters)", len(entries), afterFilters))
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

	if *metricsFlag || *metricsFile != "" {
		metricsResult.StartedAt = runStart
		metricsResult.FinishedAt = time.Now()
		printMetrics(metricsResult, *metricsFlag, *metricsFile)
	}
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
			if !query.MatchesFilters(e, query.BuildFilters(level, cutoff, search)) {
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

func applyConfig(cfg *config.Config, setFlags map[string]bool, file *string, level *string, since *string, search *string, jsonOut *bool, limit *int, output *string, tail *bool, tailFromStart *bool, tailPoll *time.Duration, format *string, storePath *string, loadPath *string, useIndex *bool, quiet *bool, storeHeader *bool, queryStr *string, explain *bool, replay *bool, snapshot *string, snapshotLoad *string, retention *string, metricsFlag *bool, metricsFile *string, serve *bool, port *int, shardDir *string, shardRead *bool, apiKey *string) {
	if !setFlags["file"] && cfg.File != nil {
		*file = *cfg.File
	}
	if !setFlags["level"] && cfg.Level != nil {
		*level = *cfg.Level
	}
	if !setFlags["since"] && cfg.Since != nil {
		*since = *cfg.Since
	}
	if !setFlags["search"] && cfg.Search != nil {
		*search = *cfg.Search
	}
	if !setFlags["json"] && cfg.JSON != nil {
		*jsonOut = *cfg.JSON
	}
	if !setFlags["limit"] && cfg.Limit != nil {
		*limit = *cfg.Limit
	}
	if !setFlags["output"] && cfg.Output != nil {
		*output = *cfg.Output
	}
	if !setFlags["tail"] && cfg.Tail != nil {
		*tail = *cfg.Tail
	}
	if !setFlags["tail-from-start"] && cfg.TailFromStart != nil {
		*tailFromStart = *cfg.TailFromStart
	}
	if !setFlags["tail-poll"] && cfg.TailPoll != nil {
		if d, err := time.ParseDuration(*cfg.TailPoll); err == nil {
			*tailPoll = d
		}
	}
	if !setFlags["format"] && cfg.Format != nil {
		*format = *cfg.Format
	}
	if !setFlags["store"] && cfg.Store != nil {
		*storePath = *cfg.Store
	}
	if !setFlags["load"] && cfg.Load != nil {
		*loadPath = *cfg.Load
	}
	if !setFlags["index"] && cfg.Index != nil {
		*useIndex = *cfg.Index
	}
	if !setFlags["quiet"] && cfg.Quiet != nil {
		*quiet = *cfg.Quiet
	}
	if !setFlags["store-header"] && cfg.StoreHeader != nil {
		*storeHeader = *cfg.StoreHeader
	}
	if !setFlags["query"] && cfg.Query != nil {
		*queryStr = *cfg.Query
	}
	if !setFlags["explain"] && cfg.Explain != nil {
		*explain = *cfg.Explain
	}
	if !setFlags["replay"] && cfg.Replay != nil {
		*replay = *cfg.Replay
	}
	if !setFlags["snapshot"] && cfg.Snapshot != nil {
		*snapshot = *cfg.Snapshot
	}
	if !setFlags["snapshot-load"] && cfg.SnapshotLoad != nil {
		*snapshotLoad = *cfg.SnapshotLoad
	}
	if !setFlags["retention"] && cfg.Retention != nil {
		*retention = *cfg.Retention
	}
	if !setFlags["metrics"] && cfg.Metrics != nil {
		*metricsFlag = *cfg.Metrics
	}
	if !setFlags["metrics-file"] && cfg.MetricsFile != nil {
		*metricsFile = *cfg.MetricsFile
	}
	if !setFlags["serve"] && cfg.Serve != nil {
		*serve = *cfg.Serve
	}
	if !setFlags["port"] && cfg.Port != nil {
		*port = *cfg.Port
	}
	if !setFlags["shard-dir"] && cfg.ShardDir != nil {
		*shardDir = *cfg.ShardDir
	}
	if !setFlags["shard-read"] && cfg.ShardRead != nil {
		*shardRead = *cfg.ShardRead
	}
	if !setFlags["api-key"] && cfg.ApiKey != nil {
		*apiKey = *cfg.ApiKey
	}
}

func buildQueryPlan(filters query.Filters, queryStr string, useIndex bool) []string {
	plan := make([]string, 0, 4)
	if useIndex {
		if filters.Level != "" {
			plan = append(plan, fmt.Sprintf("index(level=%s)", strings.ToUpper(filters.Level)))
		} else if len(filters.LevelIn) > 0 {
			plan = append(plan, fmt.Sprintf("index(level_in=%s)", strings.Join(filters.LevelIn, ",")))
		} else if !filters.After.IsZero() {
			plan = append(plan, fmt.Sprintf("index(time>=%s)", filters.After.UTC().Format(time.RFC3339)))
		} else {
			plan = append(plan, "scan(all)")
		}
	} else {
		plan = append(plan, "scan(all)")
	}

	if filters.Level != "" && !useIndex {
		plan = append(plan, fmt.Sprintf("filter(level=%s)", strings.ToUpper(filters.Level)))
	}
	if len(filters.LevelIn) > 0 {
		plan = append(plan, fmt.Sprintf("filter(level_in=%s)", strings.Join(filters.LevelIn, ",")))
	}
	if !filters.After.IsZero() {
		plan = append(plan, fmt.Sprintf("filter(after=%s)", filters.After.UTC().Format(time.RFC3339)))
	}
	if !filters.Before.IsZero() {
		plan = append(plan, fmt.Sprintf("filter(before=%s)", filters.Before.UTC().Format(time.RFC3339)))
	}
	if filters.Search != "" {
		plan = append(plan, fmt.Sprintf("filter(message~%q)", filters.Search))
	}

	if queryStr != "" {
		plan = append(plan, "dsl(parse)")
	}

	return plan
}

func printPlan(plan []string) {
	fmt.Println("PLAN:")
	for _, step := range plan {
		fmt.Printf("- %s\n", step)
	}
	fmt.Println()
}

func printMetrics(m engine.Metrics, toStdout bool, path string) {
	rate, ok := m.RatePerSec()
	rateText := "NA"
	if ok {
		rateText = fmt.Sprintf("%.2f", rate)
	}
	lines := []string{
		fmt.Sprintf("metrics.started_at=%s", m.StartedAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("metrics.finished_at=%s", m.FinishedAt.UTC().Format(time.RFC3339)),
		fmt.Sprintf("metrics.duration_ms=%d", m.Duration().Milliseconds()),
		fmt.Sprintf("metrics.logs_read=%d", m.LogsRead),
		fmt.Sprintf("metrics.logs_ingested=%d", m.LogsIngested),
		fmt.Sprintf("metrics.logs_filtered_out=%d", m.LogsFilteredOut),
		fmt.Sprintf("metrics.logs_returned=%d", m.LogsReturned),
		fmt.Sprintf("metrics.rate_per_sec=%s", rateText),
		fmt.Sprintf("metrics.index_enabled=%t", m.IndexEnabled),
	}

	if toStdout {
		fmt.Println(strings.Join(lines, "\n"))
	}

	if path != "" {
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
			log.Fatalf("failed to write metrics to %s: %v", path, err)
		}
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

func headerText(storePath string, storeHeader bool, source string) string {
	if storePath == "" || !storeHeader {
		return ""
	}
	return buildRunHeaderText(source, storePath)
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

func snapshotSources(file string, loadPath string, snapshotLoad string) []string {
	sources := make([]string, 0, 2)
	if snapshotLoad != "" {
		sources = append(sources, snapshotLoad)
	}
	if loadPath != "" {
		sources = append(sources, loadPath)
	}
	if file != "" {
		sources = append(sources, file)
	}
	return sources
}
