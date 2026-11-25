package ext

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// EventSchemaVersion is the version of the Event envelope.
const EventSchemaVersion = 1

// Event is a domain event. It carries identifiers only: no names, addresses,
// message content or notes. Extensions with the matching read permission fetch
// details through the host API.
//
// Delivery is best-effort and in-process: events live in memory, are not
// replayed after a restart, and may be dropped under backpressure.
type Event struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	SchemaVersion int    `json:"schema_version"`
	Time          string `json:"time"` // RFC 3339, UTC
	EntityType    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	Revision      int    `json:"revision,omitempty"`
	Source        string `json:"source"` // ui, cli, import, sync, extension:<id>
	CorrelationID string `json:"correlation_id,omitempty"`
}

// EventType describes one event contract.
type EventType struct {
	Type, EntityType string
	// ReadPermission is required to subscribe: the holder could read the entity anyway.
	ReadPermission string
	// Emitted is false for contracts reserved for later phases.
	Emitted bool
}

var eventTable = []EventType{
	{"contact.created", "contact", "contacts.read", true},
	{"contact.updated", "contact", "contacts.read", true},
	{"contact.archived", "contact", "contacts.read", true},
	{"contact.merged", "contact", "contacts.read", true},
	{"organization.created", "organization", "organizations.read", true},
	{"organization.updated", "organization", "organizations.read", true},
	{"activity.created", "activity", "activities.read", true},
	{"activity.updated", "activity", "activities.read", true},
	{"activity.deleted", "activity", "activities.read", true},
	{"task.created", "task", "tasks.read", true},
	{"task.updated", "task", "tasks.read", true},
	{"task.completed", "task", "tasks.read", true},
	{"task.reopened", "task", "tasks.read", true},
	{"project.created", "project", "projects.read", true},
	{"project.updated", "project", "projects.read", true},
	{"project.status_changed", "project", "projects.read", false},
	{"pipeline.created", "pipeline", "pipelines.read", true},
	{"pipeline.updated", "pipeline", "pipelines.read", true},
	{"pipeline.item_created", "pipeline_item", "pipelines.read", true},
	{"pipeline.item_moved", "pipeline_item", "pipelines.read", true},
	{"followup.due", "contact", "contacts.read", false},
	{"reminder.due", "reminder", "tasks.read", false},
}

// EventTypes lists every contract, emitted or reserved.
func EventTypes() []EventType { return append([]EventType(nil), eventTable...) }

// LookupEventType finds a contract by name.
func LookupEventType(t string) (EventType, bool) {
	for _, e := range eventTable {
		if e.Type == t {
			return e, true
		}
	}
	return EventType{}, false
}

// ExtensionSource marks events caused by an extension's own writes.
const ExtensionSource = "extension:"

// IsExtensionSource reports whether source names an extension. Such events are
// never delivered to extensions, which prevents event loops.
func IsExtensionSource(s string) bool { return strings.HasPrefix(s, ExtensionSource) }

var eventSeq atomic.Uint64

// NewID returns a unique identifier for events and correlation groups.
func NewID() string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// NewEvent fills the envelope. Revision may be zero.
func NewEvent(typ, entityID string, revision int, source, correlation string) Event {
	info, _ := LookupEventType(typ)
	eventSeq.Add(1)
	return Event{ID: NewID(), Type: typ, SchemaVersion: EventSchemaVersion, Time: time.Now().UTC().Format(time.RFC3339Nano),
		EntityType: info.EntityType, EntityID: entityID, Revision: revision, Source: source, CorrelationID: correlation}
}

// Bus fans events out to bounded subscriber queues. Publish never blocks: a
// full queue drops the new event and counts it.
type Bus struct {
	mu   sync.Mutex
	subs map[int]*Subscription
	next int
}

// Subscription is one bounded queue.
type Subscription struct {
	C       chan Event
	bus     *Bus
	id      int
	dropped atomic.Uint64
}

// NewBus returns an empty bus.
func NewBus() *Bus { return &Bus{subs: map[int]*Subscription{}} }

// Subscribe returns a queue holding up to size events.
func (b *Bus) Subscribe(size int) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &Subscription{C: make(chan Event, max(1, size)), bus: b, id: b.next}
	b.subs[b.next] = s
	b.next++
	return s
}

// Close detaches the subscription and closes its channel.
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	if _, ok := s.bus.subs[s.id]; ok {
		delete(s.bus.subs, s.id)
		close(s.C)
	}
}

// Dropped counts events lost to a full queue.
func (s *Subscription) Dropped() uint64 { return s.dropped.Load() }

// Publish offers e to every subscriber without blocking. A nil bus is a no-op,
// so callers need not check whether extensions are in use.
func (b *Bus) Publish(e Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	ids := make([]int, 0, len(b.subs))
	for id := range b.subs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		s := b.subs[id]
		select {
		case s.C <- e:
		default:
			s.dropped.Add(1)
		}
	}
	b.mu.Unlock()
}
