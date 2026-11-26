package app

import (
	"context"
	"errors"
)

// DurableEventSchemaVersion is independent of the extension protocol version.
const DurableEventSchemaVersion = 2

// IsDurableEventType reports whether a core event is recorded in the journal.
// Other event contracts can still be available on the ephemeral bus.
func IsDurableEventType(typ string) bool {
	switch typ {
	case "contact.created", "contact.updated", "contact.archived", "contact.merged",
		"organization.created", "organization.updated",
		"task.created", "task.completed", "activity.created":
		return true
	default:
		return false
	}
}

// DurableEvent identifies a committed business change. It intentionally carries
// no titles, notes, addresses, or other user-provided content.
type DurableEvent struct {
	ID            string        `json:"id"`
	Type          string        `json:"type"`
	SchemaVersion int           `json:"schema_version"`
	Source        string        `json:"source"`
	EntityType    string        `json:"entity_type"`
	EntityID      string        `json:"entity_id"`
	OccurredAt    string        `json:"occurred_at"` // RFC 3339, UTC
	RecordedAt    string        `json:"recorded_at"` // RFC 3339, UTC
	Revision      int           `json:"revision,omitempty"`
	RelatedID     string        `json:"related_id,omitempty"`
	CorrelationID string        `json:"correlation_id,omitempty"`
	CausationID   string        `json:"causation_id,omitempty"`
	AutomationID  string        `json:"automation_id,omitempty"`
	Depth         int           `json:"depth,omitempty"`
	Payload       *EventPayload `json:"payload,omitempty"`
}

// EventPayload contains enum values and record identifiers, not business text.
type EventPayload struct {
	BeforeStatus  string `json:"before_status,omitempty"`
	AfterStatus   string `json:"after_status,omitempty"`
	FromStage     string `json:"from_stage,omitempty"`
	ToStage       string `json:"to_stage,omitempty"`
	FromLifecycle string `json:"from_lifecycle,omitempty"`
	ToLifecycle   string `json:"to_lifecycle,omitempty"`
}

// EventOrigin is supplied by trusted callers, not copied from record content.
type EventOrigin struct {
	Source        string
	CorrelationID string
	CausationID   string
	AutomationID  string
	Depth         int
}

type eventOriginKey struct{}
type eventSuppressionKey struct{}

func WithEventOrigin(ctx context.Context, origin EventOrigin) context.Context {
	return context.WithValue(ctx, eventOriginKey{}, origin)
}

func EventOriginFromContext(ctx context.Context) EventOrigin {
	origin, _ := ctx.Value(eventOriginKey{}).(EventOrigin)
	return origin
}

// WithoutEvents suppresses journal entries for disposable fixture writes.
func WithoutEvents(ctx context.Context) context.Context {
	return context.WithValue(ctx, eventSuppressionKey{}, true)
}

func EventsSuppressed(ctx context.Context) bool {
	return ctx.Value(eventSuppressionKey{}) == true
}

// EventDelivery pairs a stable event with its journal sequence and the number
// of prior failed attempts in this delivery cycle.
type EventDelivery struct {
	Sequence int64        `json:"sequence"`
	Event    DurableEvent `json:"event"`
	Attempts int          `json:"attempts"`
}

// ErrEventNotEligible covers sequences that cannot be settled or replayed by
// this consumer under its current subscription.
var ErrEventNotEligible = errors.New("event is not eligible for this delivery operation")

// EventJournal delivers committed events. A nil next delivery means none is
// ready. Consumers are durable identities, and types is the current authorized
// subscription, not client-supplied permission data.
type EventJournal interface {
	NextEvent(ctx context.Context, consumer string, types []string) (*EventDelivery, error)
	SettleEvent(ctx context.Context, consumer string, types []string, sequence int64, retry bool) error
	DeadEvents(ctx context.Context, consumer string, types []string, after int64, limit int) ([]EventDelivery, error)
	ReplayEvent(ctx context.Context, consumer string, types []string, sequence int64) error
}
