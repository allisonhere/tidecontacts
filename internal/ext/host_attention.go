package ext

import (
	"context"
	"encoding/json"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

// Host calls that let an extension reuse the People Inbox, Relationship Radar's
// follow-up rules and the contact dossier instead of building its own.

// AttentionItem is one open People Inbox entry.
type AttentionItem struct {
	Key     string `json:"key"`
	Kind    string `json:"kind"` // reminder, task-overdue, followup-due, followup-suggested, task-waiting, task-inbox, date, flagged
	Title   string `json:"title"`
	Reason  string `json:"reason"`
	Due     string `json:"due,omitempty"`
	Urgency int    `json:"urgency"` // 3 overdue, 2 due now, 1 upcoming, 0 informational
	// Source is the record the entry is about; Person the one to act on.
	SourceKind string `json:"source_kind,omitempty"`
	SourceID   string `json:"source_id,omitempty"`
	PersonID   string `json:"person_id,omitempty"`
	PersonKind string `json:"person_kind,omitempty"`
	Person     string `json:"person,omitempty"`
}

// EngagementTask is an open commitment linked to a contact.
type EngagementTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Due    string `json:"due,omitempty"`
}

// EngagementDate is an upcoming important date.
type EngagementDate struct {
	Label string `json:"label"`
	Next  string `json:"next"`
}

// Engagement is what is recorded about the relationship with one contact.
// Fields are present only for permissions the extension holds; nothing is
// scored or inferred.
type Engagement struct {
	ID              string           `json:"id"`
	Kind            string           `json:"kind,omitempty"`
	Name            string           `json:"name,omitempty"`
	Missing         bool             `json:"missing,omitempty"`
	Archived        bool             `json:"archived,omitempty"`
	LastInteraction string           `json:"last_interaction,omitempty"`
	LastType        string           `json:"last_interaction_type,omitempty"`
	FollowUpDue     string           `json:"follow_up_due,omitempty"`
	FollowUpReason  string           `json:"follow_up_reason,omitempty"`
	OpenTaskCount   int              `json:"open_task_count"`
	Tasks           []EngagementTask `json:"tasks,omitempty"` // open and waiting, earliest due first
	Dates           []EngagementDate `json:"dates,omitempty"`
}

const maxEngagement = 200

func (s *Session) needAll(perms ...string) *ProtocolError {
	for _, p := range perms {
		if e := s.need(p); e != nil {
			return e
		}
	}
	return nil
}

func (s *Session) attention(ctx context.Context, method string, data json.RawMessage) (any, *ProtocolError) {
	switch method {
	case "attention.list":
		if e := s.needAll("attention.read", "contacts.read", "tasks.read"); e != nil {
			return nil, e
		}
		items, err := s.Repo.Inbox(ctx)
		if err != nil {
			return nil, mapErr(err)
		}
		out := []AttentionItem{}
		for _, it := range items {
			if it.Status != "open" {
				continue
			}
			if it.Person.Kind == "organization" && !s.Granted.Has("organizations.read") {
				continue
			}
			out = append(out, AttentionItem{Key: it.Key, Kind: it.Kind, Title: it.Title, Reason: it.Reason, Due: it.Due, Urgency: it.Urgency,
				SourceKind: it.Source.Kind, SourceID: it.Source.ID, PersonID: it.Person.ID, PersonKind: it.Person.Kind, Person: it.Person.Name})
		}
		return out, nil
	case "attention.act":
		if e := s.need("attention.write"); e != nil {
			return nil, e
		}
		var a struct {
			Key    string `json:"key"`
			Action string `json:"action"`
			Until  string `json:"until"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		if err := s.Repo.InboxAct(ctx, a.Key, a.Action, a.Until); err != nil {
			return nil, mapErr(err)
		}
		return map[string]bool{"ok": true}, nil
	case "tasks.complete":
		if e := s.need("tasks.write"); e != nil {
			return nil, e
		}
		var a struct {
			ID string `json:"id"`
		}
		if e := decode(data, &a); e != nil {
			return nil, e
		}
		t, err := s.Repo.CompleteTask(ctx, a.ID)
		if err != nil {
			return nil, mapErr(err)
		}
		return t, nil
	}
	return s.engagement(ctx, data)
}

func (s *Session) engagement(ctx context.Context, data json.RawMessage) (any, *ProtocolError) {
	if e := s.needAll("activities.read", "tasks.read"); e != nil {
		return nil, e
	}
	var a struct {
		IDs []string `json:"ids"`
	}
	if e := decode(data, &a); e != nil {
		return nil, e
	}
	if len(a.IDs) > maxEngagement {
		return nil, perr(CodeInvalid, "at most %d ids", maxEngagement)
	}
	people, orgs := s.Granted.Has("contacts.read"), s.Granted.Has("organizations.read")
	if !people && !orgs {
		return nil, perr(CodePermissionDenied, "permission contacts.read is not granted")
	}
	out := make([]Engagement, 0, len(a.IDs))
	for _, id := range a.IDs {
		if err := ctx.Err(); err != nil {
			return nil, mapErr(err)
		}
		c, err := s.Repo.Contact(ctx, id)
		allowed := err == nil && !c.Deleted && (c.Kind == "organization" && orgs || c.Kind != "organization" && people)
		if !allowed {
			out = append(out, Engagement{ID: id, Missing: true})
			continue
		}
		d, err := s.Repo.Dossier(ctx, id)
		if err != nil {
			return nil, mapErr(err)
		}
		g := Engagement{ID: id, Kind: c.Kind, Name: c.DisplayName, Archived: c.Archived, OpenTaskCount: d.OpenTaskCount,
			LastInteraction: d.FollowUp.LastInteraction, LastType: d.FollowUp.LastType, FollowUpDue: d.FollowUp.Due, FollowUpReason: d.FollowUp.Reason}
		for _, list := range [][]domain.Task{d.OpenTasks, d.Commitments} {
			for _, t := range list {
				if t.Open() {
					g.Tasks = append(g.Tasks, EngagementTask{t.ID, t.Title, t.Status, t.Due})
				}
			}
		}
		for _, x := range d.Dates {
			if x.Next != "" {
				g.Dates = append(g.Dates, EngagementDate{x.Label, x.Next})
			}
		}
		out = append(out, g)
	}
	return out, nil
}
