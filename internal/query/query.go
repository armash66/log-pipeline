package query

import (
	"fmt"
	"strings"
	"time"
)

type Filters struct {
	Level  string
	Search string
	After  time.Time
	Before time.Time
}

// Parse parses a simple AND-only query DSL.
// Supported forms:
// level=ERROR
// message~"auth"
// search~timeout
// since=10m
// after=2026-02-08T16:00:00Z
// before=2026-02-08T17:00:00Z
func Parse(input string) (Filters, error) {
	tokens, err := tokenize(input)
	if err != nil {
		return Filters{}, err
	}

	var f Filters
	for _, t := range tokens {
		key, op, val, err := splitToken(t)
		if err != nil {
			return Filters{}, err
		}

		switch strings.ToLower(key) {
		case "level":
			if op != "=" {
				return Filters{}, fmt.Errorf("level supports only '='")
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
