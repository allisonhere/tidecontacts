package app

import "context"

// RuleRecord is the bounded, note-free host projection used by local rule
// clients. Detail identifies an activity kind or the linked follow-up kind.
type RuleRecord struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Status   string   `json:"status,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Due      string   `json:"due,omitempty"`
	When     string   `json:"when,omitempty"`
	Tags     []string `json:"tags,omitzero"`
	Priority int      `json:"priority,omitempty"`
	Revision int      `json:"revision"`
}

// RuleRecordSource supports stable ID pagination, without handing out access to
// the database. The host checks the requested kind and its permission first.
type RuleRecordSource interface {
	RuleRecords(context.Context, string, string, int) ([]RuleRecord, error)
}

// FollowUpState classifies a computed due date, not a persisted event.
func FollowUpState(never bool, due, today string) string {
	switch {
	case never:
		return "never"
	case due == "":
		return "unscheduled"
	case due == today:
		return "due"
	case due < today:
		return "overdue"
	default:
		return "upcoming"
	}
}
