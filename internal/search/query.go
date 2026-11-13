// Package search parses a small, explicit search language; values may be quoted.
package search

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

type Filter struct{ Key, Value string }
type Query struct {
	Terms   []string
	Filters []Filter
}

// Parse reads a contact query. ParseScope reads the same grammar for another record type.
func Parse(s string) (Query, error) { return ParseScope("contact", s) }

func oneOf(values ...string) func(string) error {
	return func(v string) error {
		for _, x := range values {
			if strings.EqualFold(x, v) {
				return nil
			}
		}
		return fmt.Errorf("accepts %s", strings.Join(values, ", "))
	}
}
func anyText(string) error { return nil }

var window = regexp.MustCompile(`^[0-9]{1,4}[dwm]$`)

func dateWord(extra ...string) func(string) error {
	return func(v string) error {
		for _, x := range extra {
			if x == strings.ToLower(v) {
				return nil
			}
		}
		_, e := domain.ParseWhen(strings.TrimLeft(v, "<>"), "2000-01-01")
		return e
	}
}

// Scopes maps each record type to its filters. Every scope shares one lexer and one
// AND-only grammar; only the filter vocabulary differs.
var Scopes = map[string]map[string]func(string) error{
	"contact": {
		"tag": anyText, "org": anyText, "project": anyText,
		"is":       oneOf("favorite", "archived", "active", "deleted", "flagged"),
		"has":      oneOf("phone", "email", "website", "social", "messaging", "address", "tasks", "followup", "dates"),
		"followup": oneOf("due", "week", "month", "set", "none"),
		"date":     oneOf("today", "week", "month"),
		"birthday": oneOf("today", "week", "month"),
		"seen": func(v string) error {
			if v == "never" || window.MatchString(v) {
				return nil
			}
			return fmt.Errorf("accepts a window such as 30d, 8w, 6m, or never")
		},
	},
	"task": {
		"tag": anyText, "org": anyText, "project": anyText, "with": anyText,
		"status": oneOf(domain.TaskStatuses...),
		"is":     oneOf("open", "done", "overdue", "today", "recurring", "waiting", "archived", "snoozed", "unlinked"),
		"due":    dateWord("overdue", "none", "any"),
		"pri":    oneOf(domain.Priorities...),
	},
	"activity": {
		"tag": anyText, "org": anyText, "project": anyText, "with": anyText, "type": anyText, "app": anyText,
		"after": dateWord(), "before": dateWord(), "on": dateWord(),
		"source": oneOf(domain.Local, domain.Imported, domain.External),
	},
	"project": {
		"tag": anyText, "org": anyText, "with": anyText,
		"status": oneOf(domain.ProjectStatuses...),
		"is":     oneOf("open", "overdue", "waiting", "archived"),
		"pri":    oneOf(domain.Priorities...),
	},
	"item": {
		"tag": anyText, "org": anyText, "project": anyText, "with": anyText, "pipeline": anyText, "stage": anyText,
		"is": oneOf("archived", "open"),
	},
}

// Capture lists the keys understood by quick capture; they reuse filter names where one exists.
var Capture = map[string]func(string) error{
	"with": anyText, "org": anyText, "project": anyText, "tag": anyText, "type": anyText,
	"due": dateWord(), "at": anyText, "on": dateWord(), "pri": oneOf(domain.Priorities...),
	"every": anyText, "status": oneOf(domain.TaskStatuses...), "start": dateWord(), "stage": anyText,
}

// ParseCapture reads a quick-capture line: bare words form the title, key:value pairs set fields.
func ParseCapture(s string) (string, []Filter, error) {
	q, e := parse(Capture, "capture", s)
	return strings.Join(q.Terms, " "), q.Filters, e
}

func ParseScope(scope, s string) (Query, error) {
	if scope == "person" || scope == "organization" {
		scope = "contact"
	}
	keys, ok := Scopes[scope]
	if !ok {
		return Query{}, fmt.Errorf("unknown search scope %q", scope)
	}
	return parse(keys, scope, s)
}

func parse(keys map[string]func(string) error, scope, s string) (Query, error) {
	var q Query
	var token strings.Builder
	quoted, escaped := false, false
	tokens := []string{}
	flush := func() {
		if token.Len() > 0 {
			tokens = append(tokens, token.String())
			token.Reset()
		}
	}
	for _, r := range s {
		if escaped {
			token.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			continue
		}
		if unicode.IsSpace(r) && !quoted {
			flush()
		} else {
			token.WriteRune(r)
		}
	}
	if quoted {
		return q, fmt.Errorf("unclosed quote; close it with a double quote")
	}
	if escaped {
		return q, fmt.Errorf("trailing escape; add a character after the backslash")
	}
	flush()
	for _, t := range tokens {
		key, value, ok := strings.Cut(t, ":")
		if !ok {
			q.Terms = append(q.Terms, t)
			continue
		}
		key = strings.ToLower(key)
		if _, known := keys[key]; scope == "capture" && (!known || value == "") {
			q.Terms = append(q.Terms, t) // "Re: budget" and URLs stay part of the title
			continue
		}
		if value == "" {
			return q, fmt.Errorf("%s: needs a value", key)
		}
		check, ok := keys[key]
		if !ok {
			names := make([]string, 0, len(keys))
			for k := range keys {
				names = append(names, k)
			}
			sort.Strings(names)
			return q, fmt.Errorf("unknown filter %q; use %s", key, strings.Join(names, ", "))
		}
		if e := check(value); e != nil {
			return q, fmt.Errorf("%s: %s", key, e)
		}
		q.Filters = append(q.Filters, Filter{key, value})
	}
	return q, nil
}
func (q Query) FTS() string {
	parts := []string{}
	for _, s := range q.Terms {
		parts = append(parts, `"`+strings.ReplaceAll(s, `"`, `""`)+`"*`)
	}
	return strings.Join(parts, " AND ")
}

// Values reports the values of every filter with the given key.
func (q Query) Values(key string) []string {
	out := []string{}
	for _, f := range q.Filters {
		if f.Key == key {
			out = append(out, f.Value)
		}
	}
	return out
}

// Quote renders a value so that Parse reads it back as one token.
func Quote(s string) string {
	if strings.ContainsAny(s, " \t\"\\") {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	return s
}
