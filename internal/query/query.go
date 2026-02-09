package query

import (
	"fmt"
	"strings"
	"time"

	"github.com/armash/log-pipeline/internal/types"
)

type Filters struct {
	Level  string
	Search string
	After  time.Time
	Before time.Time
	Or     []Filters
	LevelIn []string
}

// Parse parses a simple query DSL with AND/OR.
// Supported forms:
// level=ERROR
// message~"auth"
// search~timeout
// since=10m
// after=2026-02-08T16:00:00Z
// before=2026-02-08T17:00:00Z
// OR is specified with: OR
// Example: level=ERROR OR level=WARN search~auth
func Parse(input string) (Filters, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return Filters{}, err
	}
	groups := splitOnOR(tokens)
	if len(groups) == 0 {
		return Filters{}, nil
	}
	if len(groups) == 1 {
		return parseAndGroup(groups[0])
	}
	root := Filters{}
	for _, g := range groups {
		f, err := parseAndGroup(g)
		if err != nil {
			return Filters{}, err
		}
		root.Or = append(root.Or, f)
	}
	return root, nil
}

func BuildFilters(level string, cutoff time.Time, search string) Filters {
	return Filters{
		Level:  level,
		Search: search,
		After:  cutoff,
	}
}

func MergeFilters(base Filters, extra Filters) (Filters, error) {
	if len(extra.Or) > 0 {
		if isEmptyFilters(base) {
			return extra, nil
		}
		root := Filters{Or: make([]Filters, 0, len(extra.Or))}
		for _, opt := range extra.Or {
			mergedOpt, err := MergeFilters(base, opt)
			if err != nil {
				return Filters{}, err
			}
			root.Or = append(root.Or, mergedOpt)
		}
		return root, nil
	}

	merged := base
	if len(extra.LevelIn) > 0 {
		if merged.Level != "" || len(merged.LevelIn) > 0 {
			return Filters{}, fmt.Errorf("conflicting level filters")
		}
		merged.LevelIn = append(merged.LevelIn, extra.LevelIn...)
	}
	if extra.Level != "" {
		if merged.Level != "" && !strings.EqualFold(merged.Level, extra.Level) {
			return Filters{}, fmt.Errorf("conflicting level filters")
		}
		if len(merged.LevelIn) > 0 {
			return Filters{}, fmt.Errorf("conflicting level filters")
		}
		merged.Level = extra.Level
	}
	if extra.Search != "" {
		if merged.Search != "" && merged.Search != extra.Search {
			return Filters{}, fmt.Errorf("conflicting search filters")
		}
		merged.Search = extra.Search
	}
	if !extra.After.IsZero() {
		if !merged.After.IsZero() && extra.After.After(merged.After) {
			merged.After = extra.After
		} else if merged.After.IsZero() {
			merged.After = extra.After
		}
	}
	if !extra.Before.IsZero() {
		if !merged.Before.IsZero() && extra.Before.Before(merged.Before) {
			merged.Before = extra.Before
		} else if merged.Before.IsZero() {
			merged.Before = extra.Before
		}
	}
	return merged, nil
}

func isEmptyFilters(f Filters) bool {
	return f.Level == "" && f.Search == "" && f.After.IsZero() && f.Before.IsZero() && len(f.LevelIn) == 0 && len(f.Or) == 0
}

func MatchesFilters(e types.LogEntry, f Filters) bool {
	if len(f.Or) > 0 {
		for _, opt := range f.Or {
			if MatchesFilters(e, opt) {
				return true
			}
		}
		return false
	}
	if len(f.LevelIn) > 0 {
		ok := false
		for _, lvl := range f.LevelIn {
			if strings.EqualFold(e.Level, lvl) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.Level != "" && !strings.EqualFold(e.Level, f.Level) {
		return false
	}
	if !f.After.IsZero() && e.Timestamp.Before(f.After) {
		return false
	}
	if !f.Before.IsZero() && !e.Timestamp.Before(f.Before) {
		return false
	}
	if f.Search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(f.Search)) {
		return false
	}
	return true
}

func parseAndGroup(tokens []string) (Filters, error) {
	var f Filters
	for _, t := range tokens {
		key, op, val, err := splitToken(t)
		if err != nil {
			return Filters{}, err
		}

		switch strings.ToLower(key) {
		case "level":
			if op == "in" {
				levels, err := parseInList(val)
				if err != nil {
					return Filters{}, err
				}
				f.LevelIn = append(f.LevelIn, levels...)
				continue
			}
			if op != "=" {
				return Filters{}, fmt.Errorf("level supports only '=' or 'in'")
			}
			f.Level = val
		case "message", "search":
			if op != "~" && op != "=" {
				return Filters{}, fmt.Errorf("message/search supports '~' or '='")
			}
			f.Search = val
		case "since":
			if op != "=" {
				return Filters{}, fmt.Errorf("since supports only '='")
			}
			d, err := time.ParseDuration(val)
			if err != nil {
				return Filters{}, fmt.Errorf("invalid since duration")
			}
			f.After = time.Now().Add(-d)
		case "after":
			if op != "=" {
				return Filters{}, fmt.Errorf("after supports only '='")
			}
			tm, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return Filters{}, fmt.Errorf("invalid after timestamp")
			}
			f.After = tm
		case "before":
			if op != "=" {
				return Filters{}, fmt.Errorf("before supports only '='")
			}
			tm, err := time.Parse(time.RFC3339, val)
			if err != nil {
				return Filters{}, fmt.Errorf("invalid before timestamp")
			}
			f.Before = tm
		default:
			return Filters{}, fmt.Errorf("unknown filter: %s", key)
		}
	}
	return f, nil
}

func splitToken(token string) (string, string, string, error) {
	lower := strings.ToLower(token)
	if strings.Contains(lower, " in ") {
		idx := strings.Index(lower, " in ")
		key := strings.TrimSpace(token[:idx])
		val := strings.TrimSpace(token[idx+4:])
		if key == "" || val == "" {
			return "", "", "", fmt.Errorf("invalid token: %s", token)
		}
		return key, "in", strings.TrimSpace(val), nil
	}

	var op string
	var idx int
	if strings.Contains(token, "~") {
		op = "~"
		idx = strings.Index(token, "~")
	} else if strings.Contains(token, "=") {
		op = "="
		idx = strings.Index(token, "=")
	} else {
		return "", "", "", fmt.Errorf("expected key=value or key~value")
	}

	key := strings.TrimSpace(token[:idx])
	val := strings.TrimSpace(token[idx+1:])
	if key == "" || val == "" {
		return "", "", "", fmt.Errorf("invalid token: %s", token)
	}
	return key, op, trimQuotes(val), nil
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func tokenize(input string) ([]string, error) {
	var tokens []string
	var b strings.Builder
	inQuote := byte(0)

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			} else {
				b.WriteByte(ch)
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			inQuote = ch
			continue
		}

		if ch == ' ' || ch == '\t' {
			if b.Len() > 0 {
				tokens = append(tokens, b.String())
				b.Reset()
			}
			continue
		}

		b.WriteByte(ch)
	}

	if inQuote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if b.Len() > 0 {
		tokens = append(tokens, b.String())
	}
	return tokens, nil
}

func splitOnOR(tokens []string) [][]string {
	groups := make([][]string, 0)
	current := make([]string, 0)
	i := 0
	for i < len(tokens) {
		t := tokens[i]
		if strings.EqualFold(t, "OR") {
			if len(current) > 0 {
				groups = append(groups, current)
				current = make([]string, 0)
			}
			i++
			continue
		}

		if i+2 < len(tokens) && strings.EqualFold(tokens[i+1], "in") {
			current = append(current, t+" in "+tokens[i+2])
			i += 3
			continue
		}

		current = append(current, t)
		i++
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func parseInList(val string) ([]string, error) {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "(") && strings.HasSuffix(val, ")") {
		val = strings.TrimPrefix(val, "(")
		val = strings.TrimSuffix(val, ")")
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		item := trimQuotes(strings.TrimSpace(p))
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty in() list")
	}
	return out, nil
}
