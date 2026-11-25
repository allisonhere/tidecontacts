package ext

import "sort"

// Enforcement says where a permission is actually checked.
type Enforcement string

const (
	// EnforcedByHost permissions gate a host API call or host behaviour.
	EnforcedByHost Enforcement = "host-api"
	// DeclaredOnly permissions are recorded and shown to the user but the host
	// cannot enforce them; a same-user subprocess keeps OS-level access.
	DeclaredOnly Enforcement = "declared-only"
	// Reserved permissions are valid names whose host API does not exist yet.
	Reserved Enforcement = "reserved"
)

// Permission describes one grantable permission.
type Permission struct {
	ID, Description string
	Enforcement     Enforcement
	Sensitive       bool // exposes personal data or mutates records
}

var permissionTable = []Permission{
	{"contacts.read", "Read people", EnforcedByHost, true},
	{"contacts.write", "Create and edit people", EnforcedByHost, true},
	{"organizations.read", "Read organizations", EnforcedByHost, true},
	{"organizations.write", "Create and edit organizations", EnforcedByHost, true},
	{"relationships.read", "Read relationships and connections", EnforcedByHost, true},
	{"relationships.write", "Create and edit relationships", Reserved, true},
	{"activities.read", "Read the activity timeline", EnforcedByHost, true},
	{"activities.write", "Log and edit activities", EnforcedByHost, true},
	{"tasks.read", "Read tasks", EnforcedByHost, true},
	{"tasks.write", "Create and edit tasks", EnforcedByHost, true},
	{"projects.read", "Read projects", EnforcedByHost, true},
	{"projects.write", "Create and edit projects", Reserved, true},
	{"pipelines.read", "Read pipelines and their items", EnforcedByHost, true},
	{"pipelines.write", "Create and edit pipelines", Reserved, true},
	{"events.subscribe", "Receive domain events (identifiers only)", EnforcedByHost, false},
	{"ui.register", "Contribute host-rendered panels", EnforcedByHost, false},
	{"commands.register", "Add command-palette actions", EnforcedByHost, false},
	{"storage.extension", "Use the extension's private key-value storage", EnforcedByHost, false},
	{"network.access", "Declares that the extension uses the network (not enforceable by the host)", DeclaredOnly, true},
}

// Permissions lists every known permission in a stable order.
func Permissions() []Permission { return append([]Permission(nil), permissionTable...) }

// LookupPermission returns the descriptor for id.
func LookupPermission(id string) (Permission, bool) {
	for _, p := range permissionTable {
		if p.ID == id {
			return p, true
		}
	}
	return Permission{}, false
}

// PermissionSet is an immutable-by-convention set of permission ids.
type PermissionSet map[string]bool

// NewPermissionSet builds a set from ids.
func NewPermissionSet(ids ...string) PermissionSet {
	s := PermissionSet{}
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// Has reports membership; a nil set denies everything (default deny).
func (s PermissionSet) Has(id string) bool { return s != nil && s[id] }

// Sorted returns the ids in order.
func (s PermissionSet) Sorted() []string {
	out := make([]string, 0, len(s))
	for id := range s {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Missing returns the ids in want that s lacks.
func (s PermissionSet) Missing(want []string) []string {
	var out []string
	for _, id := range want {
		if !s.Has(id) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
