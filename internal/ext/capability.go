package ext

import (
	"fmt"
	"sort"
	"strings"
)

// Capability is one discoverable feature.
type Capability struct {
	ID          string   `json:"id"`
	Version     int      `json:"version"`
	Provider    string   `json:"provider"` // "core" or an extension id
	Available   bool     `json:"available"`
	Reason      string   `json:"reason,omitempty"` // why it is unavailable
	Permissions []string `json:"permissions,omitempty"`
	Entitlement string   `json:"entitlement,omitempty"`
}

// CoreProvider is the provider id of built-in capabilities.
const CoreProvider = "core"

// CoreCapabilities are the Community capabilities, always available and never
// replaceable by an extension.
func CoreCapabilities() []Capability {
	c := func(id string, perms ...string) Capability {
		return Capability{ID: id, Version: 1, Provider: CoreProvider, Available: true, Permissions: perms}
	}
	return []Capability{
		c("core.contacts", "contacts.read", "contacts.write"),
		c("core.organizations", "organizations.read", "organizations.write"),
		c("core.relationships", "relationships.read", "relationships.write"),
		c("core.activities", "activities.read", "activities.write"),
		c("core.tasks", "tasks.read", "tasks.write"),
		c("core.projects", "projects.read", "projects.write"),
		c("core.pipelines", "pipelines.read", "pipelines.write"),
		c("core.insights", "contacts.read", "activities.read", "tasks.read"),
		c("core.events", "events.subscribe"),
		c("core.event-journal", "events.consume"),
		c("core.rule-records", "contacts.tags.write", "organizations.tags.write", "reminders.write"),
		c("core.mcp", "mcp.register"),
	}
}

// Registry resolves capability offers and requirements. It is rebuilt from
// scratch on each discovery, so it never holds stale providers.
type Registry struct {
	caps map[string]Capability
}

// NewRegistry seeds the registry with the core capabilities.
func NewRegistry() *Registry {
	r := &Registry{caps: map[string]Capability{}}
	for _, c := range CoreCapabilities() {
		r.caps[c.ID] = c
	}
	return r
}

// Offer registers a capability from an extension. Core ids can never be
// replaced, and an id has exactly one provider.
func (r *Registry) Offer(c Capability) error {
	if strings.HasPrefix(c.ID, "core.") || c.Provider == CoreProvider {
		return fmt.Errorf("capability %q is reserved for the core", c.ID)
	}
	if old, ok := r.caps[c.ID]; ok {
		return fmt.Errorf("capability %q is already provided by %s", c.ID, old.Provider)
	}
	r.caps[c.ID] = c
	return nil
}

// Get returns a capability by id.
func (r *Registry) Get(id string) (Capability, bool) { c, ok := r.caps[id]; return c, ok }

// Require checks that id is available at minVersion.
func (r *Registry) Require(id string, minVersion int) error {
	c, ok := r.caps[id]
	switch {
	case !ok:
		return fmt.Errorf("capability %s is not available", id)
	case !c.Available:
		return fmt.Errorf("capability %s is unavailable: %s", id, c.Reason)
	case c.Version < minVersion:
		return fmt.Errorf("capability %s version %d is older than required %d", id, c.Version, minVersion)
	}
	return nil
}

// SetAvailable marks an offered capability usable or not.
func (r *Registry) SetAvailable(id string, ok bool, reason string) {
	if c, found := r.caps[id]; found && c.Provider != CoreProvider {
		c.Available, c.Reason = ok, reason
		r.caps[id] = c
	}
}

// List returns every capability sorted by id.
func (r *Registry) List() []Capability {
	out := make([]Capability, 0, len(r.caps))
	for _, c := range r.caps {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
