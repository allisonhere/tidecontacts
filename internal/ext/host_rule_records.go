package ext

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
)

func (s *Session) ruleRecords(ctx context.Context, data json.RawMessage) (any, *ProtocolError) {
	var a struct {
		Kind  string `json:"kind"`
		After string `json:"after"`
		Limit int    `json:"limit"`
	}
	if e := decode(data, &a); e != nil {
		return nil, e
	}
	perm := map[string]string{"contact": "contacts.read", "organization": "organizations.read", "task": "tasks.read", "activity": "activities.read", "followup": "contacts.read"}[a.Kind]
	if perm == "" || len(a.After) > 256 || a.Limit < 1 || a.Limit > 500 {
		return nil, perr(CodeInvalid, "invalid record kind, cursor, or limit")
	}
	if e := s.need(perm); e != nil {
		return nil, e
	}
	if a.Kind == "followup" {
		if e := s.need("organizations.read"); e != nil {
			return nil, e
		}
	}
	r, ok := s.Repo.(app.RuleRecordSource)
	if !ok {
		return nil, perr(CodeUnknownMethod, "record paging is unavailable")
	}
	out, err := r.RuleRecords(ctx, a.Kind, a.After, a.Limit)
	return out, mapErr(err)
}

func (s *Session) permissions(data json.RawMessage) (any, *ProtocolError) {
	var a struct {
		Names []string `json:"permissions"`
	}
	if e := decode(data, &a); e != nil {
		return nil, e
	}
	if len(a.Names) > 32 {
		return nil, perr(CodeInvalid, "at most 32 permission names")
	}
	out := make(map[string]bool, len(a.Names))
	for _, name := range a.Names {
		if _, ok := LookupPermission(name); !ok {
			return nil, perr(CodeInvalid, "unknown permission name")
		}
		out[name] = s.Granted.Has(name)
	}
	return out, nil
}

func (s *Session) ruleWrite(ctx context.Context, data json.RawMessage) (any, *ProtocolError) {
	var a struct {
		Kind          string `json:"kind"`
		RecordKind    string `json:"record_kind"`
		RecordID      string `json:"record_id"`
		Title         string `json:"title"`
		Due           string `json:"due"`
		Tag           string `json:"tag"`
		Revision      int    `json:"revision"`
		CorrelationID string `json:"correlation_id"`
		CausationID   string `json:"causation_id"`
		AutomationID  string `json:"automation_id"`
		Depth         int    `json:"depth"`
	}
	if e := decode(data, &a); e != nil {
		return nil, e
	}
	ctx, e := s.ruleOrigin(ctx, a.CorrelationID, a.CausationID, a.AutomationID, a.Depth)
	if e != nil {
		return nil, e
	}
	var read, tagWrite string
	switch a.RecordKind {
	case "contact":
		read, tagWrite = "contacts.read", "contacts.tags.write"
	case "organization":
		read, tagWrite = "organizations.read", "organizations.tags.write"
	case "":
		if a.RecordID != "" {
			return nil, perr(CodeInvalid, "a linked record requires a kind")
		}
	default:
		return nil, perr(CodeInvalid, "a linked record must be a contact or organization")
	}
	perm := ""
	switch a.Kind {
	case "task":
		perm = "tasks.write"
	case "timeline":
		perm = "activities.write"
	case "add_tag", "remove_tag":
		perm = tagWrite
	}
	if perm == "" {
		return nil, perr(CodeInvalid, "unsupported record action or missing destination")
	}
	if e := s.need(perm); e != nil {
		return nil, e
	}
	var c domain.Contact
	var links []domain.Ref
	if read != "" {
		if e := s.need(read); e != nil {
			return nil, e
		}
		var err error
		c, err = s.Repo.Contact(ctx, a.RecordID)
		if err != nil || c.Deleted || c.Archived || (c.Kind == "organization") != (a.RecordKind == "organization") {
			return nil, perr(CodeNotFound, "linked record is not live")
		}
		links = []domain.Ref{{ID: c.ID, Kind: c.Kind}}
	}
	if a.Kind == "add_tag" || a.Kind == "remove_tag" {
		if a.Tag = strings.TrimSpace(a.Tag); a.Tag == "" || Clean(a.Tag, 80, false) != a.Tag || a.Revision <= 0 {
			return nil, perr(CodeInvalid, "a printable tag and positive revision are required")
		}
		if a.Revision != c.Revision {
			return nil, perr(CodeConflict, "record changed; preview the rule again")
		}
		same := func(t string) bool { return strings.EqualFold(t, a.Tag) }
		has := slices.ContainsFunc(c.Tags, same)
		if a.Kind == "add_tag" && !has {
			c.Tags = append(c.Tags, a.Tag)
		} else if a.Kind == "remove_tag" && has {
			c.Tags = slices.DeleteFunc(c.Tags, same)
		} else {
			return map[string]string{"id": c.ID}, nil
		}
		_, err := s.Repo.SaveContacts(ctx, []domain.Contact{c}, ExtensionSource+s.ID)
		return map[string]string{"id": c.ID}, mapErr(err)
	}
	if a.Title = strings.TrimSpace(a.Title); a.Title == "" || Clean(a.Title, 240, false) != a.Title {
		return nil, perr(CodeInvalid, "a printable title is required")
	}
	if a.Kind == "task" {
		t, err := s.Repo.SaveTask(ctx, domain.Task{Title: a.Title, Due: a.Due, Status: "next", Entities: links})
		return map[string]string{"id": t.ID}, mapErr(err)
	}
	r, err := s.Repo.SaveActivity(ctx, domain.Activity{Type: "automation", Title: a.Title, Entities: links, Provenance: domain.Local, SourceApp: ExtensionSource + s.ID, OccurredAt: time.Now()})
	return map[string]string{"id": r.ID}, mapErr(err)
}

func (s *Session) ruleOrigin(ctx context.Context, correlation, causation, automation string, depth int) (context.Context, *ProtocolError) {
	if !idToken.MatchString(automation) || !idToken.MatchString(correlation) || causation != "" && !idToken.MatchString(causation) || depth < 1 || depth > 8 {
		return ctx, perr(CodeInvalid, "invalid action provenance")
	}
	return app.WithEventOrigin(ctx, app.EventOrigin{Source: ExtensionSource + s.ID, CorrelationID: correlation, CausationID: causation, AutomationID: automation, Depth: depth}), nil
}
