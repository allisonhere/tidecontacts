package ext

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/allisonhere/tidecontacts/internal/app"
)

// ManifestFile is the fixed manifest name inside an extension directory.
const ManifestFile = "extension.toml"

const maxManifestBytes = 64 << 10

// Manifest is the declarative description of an extension. A valid manifest is
// not proof that the extension is trusted: it only states what is requested.
type Manifest struct {
	ID          string `toml:"id" json:"id"`
	Name        string `toml:"name" json:"name"`
	Version     string `toml:"version" json:"version"`
	API         int    `toml:"api" json:"api"` // extension API major version
	Description string `toml:"description" json:"description,omitempty"`
	Author      string `toml:"author" json:"author,omitempty"`
	// Command is the executable, relative to the extension directory. It is run
	// directly, never through a shell, and receives no arguments.
	Command     string   `toml:"command" json:"command"`
	MinHost     string   `toml:"min_host" json:"min_host,omitempty"`
	Requires    []string `toml:"requires" json:"requires,omitempty"`
	Optional    []string `toml:"optional" json:"optional,omitempty"`
	Permissions []string `toml:"permissions" json:"permissions,omitempty"`
	// Entitlement optionally names a capability the entitlement provider must
	// allow before the extension may run.
	Entitlement string         `toml:"entitlement" json:"entitlement,omitempty"`
	Provides    []Provided     `toml:"provides" json:"provides,omitempty"`
	Commands    []CommandDecl  `toml:"commands" json:"commands,omitempty"`
	Panels      []PanelDecl    `toml:"panels" json:"panels,omitempty"`
	Events      EventDecl      `toml:"events" json:"events"`
	Config      []ConfigOption `toml:"config" json:"config,omitempty"`
	Storage     StorageDecl    `toml:"storage" json:"storage"`
	// Additive API 1 contributions.
	Workspaces []WorkspaceDecl  `toml:"workspaces" json:"workspaces,omitempty"`
	CLI        []CLIDecl        `toml:"cli" json:"cli,omitempty"`
	Signals    bool             `toml:"signals" json:"signals,omitempty"`
	Published  []PublishedEvent `toml:"published_events" json:"published_events,omitempty"`
	// TimeoutSeconds raises the per-request budget (default 10, host cap 30).
	TimeoutSeconds int `toml:"timeout_seconds" json:"timeout_seconds,omitempty"`
}

// WorkspaceDecl contributes a full-screen workspace, listed in the menu only
// while the extension is enabled. The host renders what the extension describes.
type WorkspaceDecl struct {
	ID    string `toml:"id" json:"id"`
	Title string `toml:"title" json:"title"`
}

// CLIDecl contributes a top-level JSON CLI resource served by the extension.
type CLIDecl struct {
	Resource    string   `toml:"resource" json:"resource"`
	Description string   `toml:"description" json:"description,omitempty"`
	Actions     []string `toml:"actions" json:"actions,omitempty"`
}

// PublishedEvent declares an event type the extension may publish through the
// host. The payload is identifiers only; the host stamps the source.
type PublishedEvent struct {
	Type       string `toml:"type" json:"type"`
	EntityType string `toml:"entity_type" json:"entity_type"`
}

// ReservedCLIResources are built-in names an extension cannot take over.
var ReservedCLIResources = map[string]bool{"contacts": true, "organizations": true, "tasks": true, "activities": true, "projects": true, "pipelines": true, "items": true, "crm": true, "schema": true, "extensions": true, "capabilities": true, "help": true}

// Provided is a capability an extension offers.
type Provided struct {
	ID      string `toml:"id" json:"id"`
	Version int    `toml:"version" json:"version"`
}

// CommandDecl contributes one command-palette action. Extensions cannot bind keys.
type CommandDecl struct {
	ID          string `toml:"id" json:"id"`
	Title       string `toml:"title" json:"title"`
	Description string `toml:"description" json:"description,omitempty"`
}

// PanelDecl contributes one host-rendered panel into a named slot.
type PanelDecl struct {
	ID    string `toml:"id" json:"id"`
	Title string `toml:"title" json:"title"`
	Slot  string `toml:"slot" json:"slot"`
}

// EventDecl lists subscribed event types.
type EventDecl struct {
	Subscribe []string `toml:"subscribe" json:"subscribe,omitempty"`
	Durable   []string `toml:"durable" json:"durable,omitempty"`
}

// StorageDecl states whether private storage is needed and how much.
type StorageDecl struct {
	Required bool `toml:"required" json:"required"`
	MaxKB    int  `toml:"max_kb" json:"max_kb,omitempty"`
	// SQL asks for a private directory (TIDECONTACTS_EXTENSION_DATA) in which the
	// extension keeps its own SQLite database. It needs storage.sql.
	SQL bool `toml:"sql" json:"sql,omitempty"`
}

// ConfigOption describes one user setting. Values live in host state, not in
// the extension directory.
type ConfigOption struct {
	Key     string   `toml:"key" json:"key"`
	Label   string   `toml:"label" json:"label"`
	Type    string   `toml:"type" json:"type"` // string, integer, boolean, enum
	Default any      `toml:"default" json:"default,omitempty"`
	Options []string `toml:"options" json:"options,omitempty"`
	Min     *int64   `toml:"min" json:"min,omitempty"`
	Max     *int64   `toml:"max" json:"max,omitempty"`
	Help    string   `toml:"help" json:"help,omitempty"`
}

// Panel slots the host currently renders. Other planned slots (navigation,
// dashboard, settings, search providers) are deliberately rejected until the
// host implements them.
const SlotContactDossier = "contact.dossier"

var (
	idPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	capPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)+$`)
	localPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	keyPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
)

// ValidID reports whether id is a legal extension identifier.
func ValidID(id string) bool { return len(id) >= 3 && len(id) <= 48 && idPattern.MatchString(id) }

// LoadManifest reads and validates dir/extension.toml.
func LoadManifest(dir string) (Manifest, error) {
	var m Manifest
	path := filepath.Join(dir, ManifestFile)
	st, err := os.Lstat(path)
	if err != nil {
		return m, fmt.Errorf("manifest: %w", err)
	}
	if !st.Mode().IsRegular() {
		return m, fmt.Errorf("manifest must be a regular file")
	}
	if st.Size() > maxManifestBytes {
		return m, fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return m, err
	}
	return ParseManifest(data)
}

// ParseManifest decodes and validates manifest bytes. Unknown keys are errors.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) > maxManifestBytes {
		return m, fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	if !utf8.Valid(data) {
		return m, fmt.Errorf("manifest is not valid UTF-8")
	}
	meta, err := toml.Decode(string(data), &m)
	if err != nil {
		return m, fmt.Errorf("manifest: %w", err)
	}
	if u := meta.Undecoded(); len(u) > 0 {
		return m, fmt.Errorf("manifest: unknown key %s", u[0])
	}
	return m, m.Validate()
}

func text(label, v string, max int, required bool) error {
	if required && strings.TrimSpace(v) == "" {
		return fmt.Errorf("%s is required", label)
	}
	if utf8.RuneCountInString(v) > max {
		return fmt.Errorf("%s exceeds %d characters", label, max)
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return fmt.Errorf("%s contains control characters", label)
		}
	}
	return nil
}

func dupes(label string, ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("duplicate %s %q", label, id)
		}
		seen[id] = true
	}
	return nil
}

// Validate checks structure, not trust.
func (m Manifest) Validate() error {
	if !ValidID(m.ID) {
		return fmt.Errorf("id %q must be 3-48 lowercase letters, digits and single hyphens, starting with a letter", m.ID)
	}
	for _, c := range []struct {
		l, v string
		n    int
		req  bool
	}{{"name", m.Name, 60, true}, {"version", m.Version, 32, true}, {"description", m.Description, 400, false}, {"author", m.Author, 80, false}} {
		if err := text(c.l, c.v, c.n, c.req); err != nil {
			return err
		}
	}
	if _, ok := parseVersion(m.Version); !ok {
		return fmt.Errorf("version %q must look like 1.2.3", m.Version)
	}
	if m.API <= 0 {
		return fmt.Errorf("api is required")
	}
	if m.MinHost != "" {
		if _, ok := parseVersion(m.MinHost); !ok {
			return fmt.Errorf("min_host %q must look like 0.3.0", m.MinHost)
		}
	}
	if err := ValidateCommandPath(m.Command); err != nil {
		return err
	}
	for _, id := range append(append([]string{}, m.Requires...), m.Optional...) {
		if !capPattern.MatchString(id) {
			return fmt.Errorf("capability %q is malformed", id)
		}
	}
	if err := dupes("capability requirement", append(append([]string{}, m.Requires...), m.Optional...)); err != nil {
		return err
	}
	if err := dupes("permission", m.Permissions); err != nil {
		return err
	}
	granted := NewPermissionSet(m.Permissions...)
	for _, p := range m.Permissions {
		if _, ok := LookupPermission(p); !ok {
			return fmt.Errorf("unknown permission %q", p)
		}
	}
	if m.Entitlement != "" && !capPattern.MatchString(m.Entitlement) {
		return fmt.Errorf("entitlement %q is malformed", m.Entitlement)
	}
	var provided []string
	for _, p := range m.Provides {
		if !capPattern.MatchString(p.ID) || p.Version < 1 {
			return fmt.Errorf("provided capability %q needs a dotted id and version >= 1", p.ID)
		}
		if strings.HasPrefix(p.ID, "core.") {
			return fmt.Errorf("capability %q is in the reserved core namespace", p.ID)
		}
		if !strings.HasPrefix(p.ID, m.ID+".") && !strings.HasPrefix(p.ID, "pro.") {
			return fmt.Errorf("capability %q must start with %q or the entitlement-gated pro. namespace", p.ID, m.ID+".")
		}
		provided = append(provided, p.ID)
	}
	if err := dupes("provided capability", provided); err != nil {
		return err
	}
	var cmds, panels []string
	if len(m.Commands) > 0 && !granted.Has("commands.register") {
		return fmt.Errorf("commands require the commands.register permission")
	}
	for _, c := range m.Commands {
		if !localPattern.MatchString(c.ID) {
			return fmt.Errorf("command id %q is malformed", c.ID)
		}
		if err := text("command title", c.Title, 60, true); err != nil {
			return err
		}
		if err := text("command description", c.Description, 200, false); err != nil {
			return err
		}
		cmds = append(cmds, c.ID)
	}
	if len(cmds) > 16 {
		return fmt.Errorf("at most 16 commands")
	}
	if err := dupes("command", cmds); err != nil {
		return err
	}
	if len(m.Panels) > 0 && !granted.Has("ui.register") {
		return fmt.Errorf("panels require the ui.register permission")
	}
	for _, p := range m.Panels {
		if !localPattern.MatchString(p.ID) {
			return fmt.Errorf("panel id %q is malformed", p.ID)
		}
		if err := text("panel title", p.Title, 40, true); err != nil {
			return err
		}
		if p.Slot != SlotContactDossier {
			return fmt.Errorf("panel %q: unsupported slot %q (supported: %s)", p.ID, p.Slot, SlotContactDossier)
		}
		panels = append(panels, p.ID)
	}
	if len(m.Panels) > 4 {
		return fmt.Errorf("at most 4 panels")
	}
	if err := dupes("panel", panels); err != nil {
		return err
	}
	if len(m.Events.Subscribe) > 0 && !granted.Has("events.subscribe") {
		return fmt.Errorf("event subscriptions require the events.subscribe permission")
	}
	for _, t := range m.Events.Subscribe {
		info, ok := LookupEventType(t)
		if !ok {
			return fmt.Errorf("unknown event type %q", t)
		}
		if !granted.Has(info.ReadPermission) {
			return fmt.Errorf("event %q requires the %s permission", t, info.ReadPermission)
		}
	}
	if err := dupes("event subscription", m.Events.Subscribe); err != nil {
		return err
	}
	if len(m.Events.Durable) > 0 && !granted.Has("events.consume") {
		return fmt.Errorf("durable subscriptions require the events.consume permission")
	}
	for _, t := range m.Events.Durable {
		info, ok := LookupEventType(t)
		if !ok || !app.IsDurableEventType(t) {
			return fmt.Errorf("event %q is not a durable topic", t)
		}
		if !granted.Has(info.ReadPermission) {
			return fmt.Errorf("event %q requires the %s permission", t, info.ReadPermission)
		}
	}
	if err := dupes("durable event subscription", m.Events.Durable); err != nil {
		return err
	}
	if m.Storage.SQL && !granted.Has("storage.sql") {
		return fmt.Errorf("an SQL data directory requires the storage.sql permission")
	}
	if err := m.validateContributions(granted); err != nil {
		return err
	}
	if (m.Storage.Required || m.Storage.MaxKB != 0) && !granted.Has("storage.extension") {
		return fmt.Errorf("storage requires the storage.extension permission")
	}
	if m.Storage.MaxKB < 0 || m.Storage.MaxKB > MaxStorageKB {
		return fmt.Errorf("storage max_kb must be between 0 and %d", MaxStorageKB)
	}
	var keys []string
	for _, c := range m.Config {
		if !keyPattern.MatchString(c.Key) {
			return fmt.Errorf("config key %q is malformed", c.Key)
		}
		if err := text("config label", c.Label, 60, true); err != nil {
			return err
		}
		if err := text("config help", c.Help, 200, false); err != nil {
			return err
		}
		if _, err := c.Normalize(c.Default); c.Default != nil && err != nil {
			return fmt.Errorf("config %q default: %w", c.Key, err)
		}
		keys = append(keys, c.Key)
	}
	if len(m.Config) > 16 {
		return fmt.Errorf("at most 16 config options")
	}
	return dupes("config key", keys)
}

// ValidateCommandPath rejects absolute paths, traversal, separators tricks and
// empty values. Containment is re-verified against the real file system later.
func ValidateCommandPath(p string) error {
	if p == "" {
		return fmt.Errorf("command is required")
	}
	if strings.ContainsAny(p, "\x00\\") || filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return fmt.Errorf("command %q must be a relative path inside the extension directory", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == "" || part == "." {
			return fmt.Errorf("command %q must be a clean relative path", p)
		}
	}
	return text("command", p, 120, true)
}

// Normalize validates and canonicalizes a config value.
func (c ConfigOption) Normalize(v any) (any, error) {
	switch c.Type {
	case "string":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be text")
		}
		return s, text("value", s, 200, false)
	case "boolean":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("must be true or false")
		}
		return b, nil
	case "integer":
		var n int64
		switch t := v.(type) {
		case int64:
			n = t
		case int:
			n = int64(t)
		case float64:
			if t != float64(int64(t)) {
				return nil, fmt.Errorf("must be a whole number")
			}
			n = int64(t)
		default:
			return nil, fmt.Errorf("must be a whole number")
		}
		if c.Min != nil && n < *c.Min || c.Max != nil && n > *c.Max {
			return nil, fmt.Errorf("out of range")
		}
		return n, nil
	case "enum":
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be one of %s", strings.Join(c.Options, ", "))
		}
		for _, o := range c.Options {
			if o == s {
				return s, nil
			}
		}
		return nil, fmt.Errorf("must be one of %s", strings.Join(c.Options, ", "))
	}
	return nil, fmt.Errorf("unknown config type %q", c.Type)
}

// ParseConfigText converts command-line text to the option's type.
func (c ConfigOption) ParseConfigText(s string) (any, error) {
	switch c.Type {
	case "integer":
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("must be a whole number")
		}
		return c.Normalize(n)
	case "boolean":
		b, err := strconv.ParseBool(s)
		if err != nil {
			return nil, fmt.Errorf("must be true or false")
		}
		return b, nil
	}
	return c.Normalize(s)
}

// parseVersion reads "major.minor.patch" (patch optional, suffix ignored).
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	core := strings.SplitN(s, "-", 2)[0]
	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// HostAtLeast reports whether the running host satisfies min.
func HostAtLeast(min string) bool {
	if min == "" {
		return true
	}
	want, ok := parseVersion(min)
	have, _ := parseVersion(HostVersion)
	if !ok {
		return false
	}
	for i := range want {
		if have[i] != want[i] {
			return have[i] > want[i]
		}
	}
	return true
}

// SortedPermissions returns the requested permissions in stable order.
func (m Manifest) SortedPermissions() []string {
	out := append([]string(nil), m.Permissions...)
	sort.Strings(out)
	return out
}

var (
	publishedType   = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9_]*)+$`)
	publishedEntity = regexp.MustCompile(`^[a-z][a-z_]{0,29}$`)
)

func (m Manifest) validateContributions(granted PermissionSet) error {
	if m.TimeoutSeconds < 0 || m.TimeoutSeconds > int(MaxTimeout.Seconds()) {
		return fmt.Errorf("timeout_seconds must be between 0 and %d", int(MaxTimeout.Seconds()))
	}
	if len(m.Workspaces) > 0 && !granted.Has("ui.register") {
		return fmt.Errorf("workspaces require the ui.register permission")
	}
	if len(m.Workspaces) > 3 {
		return fmt.Errorf("at most 3 workspaces")
	}
	var ids []string
	for _, w := range m.Workspaces {
		if !localPattern.MatchString(w.ID) {
			return fmt.Errorf("workspace id %q is malformed", w.ID)
		}
		if err := text("workspace title", w.Title, 24, true); err != nil {
			return err
		}
		ids = append(ids, w.ID)
	}
	if err := dupes("workspace", ids); err != nil {
		return err
	}
	if len(m.CLI) > 0 && !granted.Has("cli.register") {
		return fmt.Errorf("CLI resources require the cli.register permission")
	}
	if len(m.CLI) > 16 {
		return fmt.Errorf("at most 16 CLI resources")
	}
	var res []string
	for _, c := range m.CLI {
		if !localPattern.MatchString(c.Resource) || ReservedCLIResources[c.Resource] {
			return fmt.Errorf("CLI resource %q is malformed or reserved", c.Resource)
		}
		if err := text("CLI description", c.Description, 120, false); err != nil {
			return err
		}
		if len(c.Actions) > 32 {
			return fmt.Errorf("at most 32 actions per CLI resource")
		}
		for _, a := range c.Actions {
			if !localPattern.MatchString(a) {
				return fmt.Errorf("CLI action %q is malformed", a)
			}
		}
		res = append(res, c.Resource)
	}
	if err := dupes("CLI resource", res); err != nil {
		return err
	}
	if m.Signals && !granted.Has("signals.provide") {
		return fmt.Errorf("signals require the signals.provide permission")
	}
	if len(m.Published) > 0 && !granted.Has("events.publish") {
		return fmt.Errorf("published events require the events.publish permission")
	}
	if len(m.Published) > 32 {
		return fmt.Errorf("at most 32 published events")
	}
	var pub []string
	for _, p := range m.Published {
		if !publishedType.MatchString(p.Type) || len(p.Type) > 48 || !publishedEntity.MatchString(p.EntityType) {
			return fmt.Errorf("published event %q is malformed", p.Type)
		}
		if _, core := LookupEventType(p.Type); core {
			return fmt.Errorf("published event %q collides with a core event", p.Type)
		}
		pub = append(pub, p.Type)
	}
	return dupes("published event", pub)
}
