// Command relationship-digest is the TideContacts demonstration extension.
//
// It speaks extension API 1: the host starts it once per request, writes one
// JSON line to stdin, and the extension may make host_call requests (answered
// on stdin) before it writes a single response line. It has no dependencies
// beyond the Go standard library, and no access beyond the host API.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"
)

type message struct {
	API       int             `json:"api"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Method    string          `json:"method,omitempty"`
	OK        bool            `json:"ok,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type line struct {
	Label string `json:"label,omitempty"`
	Text  string `json:"text"`
	Level string `json:"level,omitempty"`
}
type content struct {
	Title string `json:"title,omitempty"`
	Lines []line `json:"lines"`
}

var (
	in  = bufio.NewReaderSize(os.Stdin, 1<<20)
	out = json.NewEncoder(os.Stdout)
)

func read() (message, error) {
	var m message
	b, err := in.ReadBytes('\n')
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// host performs one host_call and waits for its host_response.
func host(method string, args any, result any) error {
	raw, _ := json.Marshal(args)
	if err := out.Encode(message{API: 1, Type: "host_call", CallID: "1", Method: method, Data: raw}); err != nil {
		return err
	}
	r, err := read()
	if err != nil {
		return err
	}
	if !r.OK {
		if r.Error != nil {
			return fmt.Errorf("%s: %s", r.Error.Code, r.Error.Message)
		}
		return fmt.Errorf("host call failed")
	}
	if result != nil {
		return json.Unmarshal(r.Data, result)
	}
	return nil
}

func main() {
	req, err := read()
	if err != nil || req.Type != "request" {
		fmt.Fprintln(os.Stderr, "expected a request")
		os.Exit(2)
	}
	data, herr := handle(req)
	resp := message{API: 1, Type: "response", RequestID: req.RequestID, OK: herr == nil}
	if herr != nil {
		resp.Error = &struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{"failed", herr.Error()}
	} else {
		resp.Data, _ = json.Marshal(data)
	}
	_ = out.Encode(resp)
}

func handle(req message) (any, error) {
	switch req.Method {
	case "activate", "deactivate":
		return map[string]any{}, nil
	case "health":
		return map[string]any{"ok": true, "message": "ready"}, nil
	case "event.deliver":
		return nil, countEvent(req.Data)
	case "command.run":
		var p struct{ Command string }
		_ = json.Unmarshal(req.Data, &p)
		switch p.Command {
		case "digest":
			return digest()
		case "probe":
			return probe()
		}
		return nil, fmt.Errorf("unknown command %q", p.Command)
	case "panel.render":
		var p struct {
			EntityID string `json:"entity_id"`
		}
		_ = json.Unmarshal(req.Data, &p)
		return panel(p.EntityID)
	}
	return nil, fmt.Errorf("unsupported method %q", req.Method)
}

func windowDays() int {
	var cfg map[string]any
	if err := host("config.get", map[string]any{}, &cfg); err != nil {
		return 30
	}
	if v, ok := cfg["window_days"].(float64); ok && v >= 1 {
		return int(v)
	}
	return 30
}

type summary struct{ ID, Name string }
type activity struct {
	OccurredAt time.Time
	Entities   []struct{ ID string }
}

func digest() (any, error) {
	days := windowDays()
	var people []summary
	if err := host("contacts.list", map[string]any{"limit": 500}, &people); err != nil {
		return nil, err
	}
	var acts []activity
	if err := host("activities.list", map[string]any{"limit": 500}, &acts); err != nil {
		return nil, err
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	recent := map[string]time.Time{}
	for _, a := range acts {
		for _, e := range a.Entities {
			if a.OccurredAt.After(recent[e.ID]) {
				recent[e.ID] = a.OccurredAt
			}
		}
	}
	var quiet []string
	for _, p := range people {
		if t, ok := recent[p.ID]; !ok || t.Before(cutoff) {
			quiet = append(quiet, p.Name)
		}
	}
	sort.Strings(quiet)
	c := content{Title: "Relationship digest", Lines: []line{
		{Label: "People", Text: strconv.Itoa(len(people))},
		{Label: "Window", Text: fmt.Sprintf("%d days", days)},
		{Label: "Quiet", Text: fmt.Sprintf("%d without recent activity", len(quiet)), Level: levelFor(len(quiet))},
	}}
	for i, n := range quiet {
		if i == 8 {
			c.Lines = append(c.Lines, line{Text: fmt.Sprintf("… and %d more", len(quiet)-8)})
			break
		}
		c.Lines = append(c.Lines, line{Label: "·", Text: n})
	}
	seen := "0"
	var v struct {
		Found bool
		Value string
	}
	if host("storage.get", map[string]any{"key": "events_seen"}, &v) == nil && v.Found {
		seen = v.Value
	}
	c.Lines = append(c.Lines, line{Label: "Events", Text: seen + " activity or contact events observed"})
	return c, nil
}

func levelFor(n int) string {
	if n == 0 {
		return "ok"
	}
	return "warn"
}

// probe demonstrates default-deny: this extension holds no tasks.read.
func probe() (any, error) {
	var ignored any
	err := host("tasks.list", map[string]any{}, &ignored)
	msg, level := "the host allowed an ungranted call (unexpected)", "warn"
	if err != nil {
		msg, level = err.Error(), "ok"
	}
	return content{Title: "Permission check", Lines: []line{{Label: "tasks.list", Text: msg, Level: level}}}, nil
}

func panel(entityID string) (any, error) {
	var acts []activity
	if err := host("activities.list", map[string]any{"entity_id": entityID, "limit": 200}, &acts); err != nil {
		return nil, err
	}
	last := "never"
	var latest time.Time
	for _, a := range acts {
		if a.OccurredAt.After(latest) {
			latest = a.OccurredAt
		}
	}
	if !latest.IsZero() {
		last = latest.Format("2006-01-02")
	}
	return content{Lines: []line{{Label: "Activities", Text: strconv.Itoa(len(acts))}, {Label: "Last", Text: last}}}, nil
}

func countEvent(raw json.RawMessage) error {
	var v struct {
		Found bool
		Value string
	}
	_ = host("storage.get", map[string]any{"key": "events_seen"}, &v)
	n, _ := strconv.Atoi(v.Value)
	return host("storage.put", map[string]any{"key": "events_seen", "value": strconv.Itoa(n + 1)}, nil)
}
