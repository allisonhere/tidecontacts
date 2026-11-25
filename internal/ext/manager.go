package ext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
)

// Status is an extension's lifecycle state.
type Status string

const (
	StatusDisabled      Status = "disabled"
	StatusEnabled       Status = "enabled"
	StatusFailed        Status = "failed"
	StatusInvalid       Status = "invalid"
	StatusIncompatible  Status = "incompatible"
	StatusConflict      Status = "conflict"
	StatusNeedsApproval Status = "needs-approval"
	StatusUnsatisfied   Status = "unsatisfied"
	StatusEntitlement   Status = "entitlement-required"
)

const (
	maxExtensions    = 64
	failureThreshold = 5
	eventQueueSize   = 64
	concurrentCalls  = 2
)

// ErrApprovalRequired means enabling needs the user to approve permissions.
type ApprovalRequiredError struct{ Permissions []string }

func (e *ApprovalRequiredError) Error() string {
	return "approval required for permissions: " + strings.Join(e.Permissions, ", ")
}

// Health is a sanitized running record of an extension's behaviour.
type Health struct {
	LastOK              string `json:"last_ok,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	LastErrorAt         string `json:"last_error_at,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	Invocations         int    `json:"invocations"`
	LastDurationMS      int64  `json:"last_duration_ms"`
	EventsDelivered     int    `json:"events_delivered"`
	EventsDropped       uint64 `json:"events_dropped"`
	Diagnostic          string `json:"diagnostic,omitempty"` // sanitized stderr tail
}

// PermissionInfo is one requested permission with its grant state.
type PermissionInfo struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Enforcement Enforcement `json:"enforcement"`
	Sensitive   bool        `json:"sensitive"`
	Granted     bool        `json:"granted"`
}

// ConfigInfo is a config option with its effective value.
type ConfigInfo struct {
	ConfigOption
	Value any `json:"value"`
}

// Info is the read model used by the CLI and TUI.
type Info struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Version       string           `json:"version"`
	API           int              `json:"api"`
	Description   string           `json:"description,omitempty"`
	Author        string           `json:"author,omitempty"`
	Status        Status           `json:"status"`
	StatusDetail  string           `json:"status_detail,omitempty"`
	Directory     string           `json:"directory"`
	Requires      []string         `json:"requires,omitempty"`
	Optional      []string         `json:"optional,omitempty"`
	Provides      []Provided       `json:"provides,omitempty"`
	Permissions   []PermissionInfo `json:"permissions"`
	NeedsApproval []string         `json:"needs_approval,omitempty"`
	Entitlement   string           `json:"entitlement,omitempty"`
	Commands      []CommandDecl    `json:"commands,omitempty"`
	Panels        []PanelDecl      `json:"panels,omitempty"`
	Events        []string         `json:"events,omitempty"`
	Config        []ConfigInfo     `json:"config,omitempty"`
	Storage       StorageDecl      `json:"storage"`
	Health        Health           `json:"health"`
}

type record struct {
	dir      string
	m        Manifest
	status   Status
	detail   string
	entry    stateEntry
	execPath string
	execSHA  string
	health   Health
	store    *Storage
	sem      chan struct{}
	queue    chan Event
	cancel   context.CancelFunc
	dropped  uint64
}

// Options configure a Manager.
type Options struct {
	// Dir holds one directory per extension; StateFile and DataDir are owned by the host.
	Dir, DataDir, StateFile string
	Repo                    app.CRM
	Entitlements            EntitlementProvider
	Bus                     *Bus
	Logger                  *slog.Logger
	// Timeout overrides DefaultTimeout (tests).
	Timeout time.Duration
}

// Manager discovers extensions and manages their lifecycle. A nil Manager is
// valid for callers only through the package helpers; construct one with New.
type Manager struct {
	o        Options
	mu       sync.Mutex
	recs     map[string]*record
	order    []string
	reg      *Registry
	warnings []string
	ctx      context.Context
	stop     context.CancelFunc
	wg       sync.WaitGroup
	sub      *Subscription
}

// New builds a Manager. Call Discover before use.
func New(o Options) *Manager {
	if o.Entitlements == nil {
		o.Entitlements = CommunityEntitlements{}
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(discard{}, nil))
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	m := &Manager{o: o, recs: map[string]*record{}, reg: NewRegistry()}
	m.ctx, m.stop = context.WithCancel(context.Background())
	return m
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// Discover rescans the extension directory and recomputes every status. It
// never runs extension code.
func (m *Manager) Discover(ctx context.Context) error {
	st, serr := readState(m.o.StateFile)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.warnings = nil
	if serr != nil {
		m.warnings = append(m.warnings, serr.Error())
	}
	old := m.recs
	m.recs, m.order = map[string]*record{}, nil
	entries, err := os.ReadDir(m.o.Dir)
	if err != nil && !os.IsNotExist(err) {
		m.recs = old
		return err
	}
	seen := map[string]bool{}
	count := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") { // symlinked directories are not followed
			continue
		}
		if count++; count > maxExtensions {
			m.warnings = append(m.warnings, fmt.Sprintf("only the first %d extension directories are scanned", maxExtensions))
			break
		}
		dir := filepath.Join(m.o.Dir, e.Name())
		r := &record{dir: dir, sem: make(chan struct{}, concurrentCalls)}
		man, err := LoadManifest(dir)
		key := e.Name()
		switch {
		case err != nil:
			r.m = Manifest{ID: key, Name: key}
			r.status, r.detail = StatusInvalid, Clean(err.Error(), 300, false)
		case man.ID != e.Name():
			r.m = Manifest{ID: key, Name: key}
			if seen[man.ID] {
				r.detail = fmt.Sprintf("duplicate extension id %q", man.ID)
			} else {
				r.detail = fmt.Sprintf("directory name must match the manifest id %q", man.ID)
			}
			r.status = StatusInvalid
		default:
			r.m = man
		}
		if r.status == "" {
			seen[man.ID] = true
		}
		if prev, ok := old[key]; ok && r.status == "" {
			r.health, r.dropped, r.queue, r.cancel = prev.health, prev.dropped, prev.queue, prev.cancel
		}
		m.recs[key] = r
		m.order = append(m.order, key)
	}
	sort.Strings(m.order)
	m.resolveLocked(st)
	return nil
}

// resolveLocked derives statuses, the registry and activation candidates.
func (m *Manager) resolveLocked(st stateFile) {
	m.reg = NewRegistry()
	ctx := context.Background()
	for _, id := range m.order {
		r := m.recs[id]
		if r.status != "" {
			continue
		}
		r.entry = st.Extensions[id]
		r.store, _ = OpenStorage(m.dataRoot(), id, r.m.Storage.MaxKB)
		switch {
		case r.m.API != APIVersion:
			r.status, r.detail = StatusIncompatible, fmt.Sprintf("needs extension API %d; this host speaks %d", r.m.API, APIVersion)
		case !HostAtLeast(r.m.MinHost):
			r.status, r.detail = StatusIncompatible, fmt.Sprintf("needs TideContacts %s or newer (this is %s)", r.m.MinHost, HostVersion)
		default:
			path, err := ResolveExecutable(r.dir, r.m.Command)
			if err != nil {
				r.status, r.detail = StatusInvalid, Clean(err.Error(), 300, false)
			} else {
				r.execPath = path
			}
		}
	}
	// Offer provided capabilities: enabled extensions first so a disabled one
	// cannot shadow an active provider.
	ids := append([]string(nil), m.order...)
	sort.SliceStable(ids, func(i, j int) bool { return m.recs[ids[i]].entry.Enabled && !m.recs[ids[j]].entry.Enabled })
	for _, id := range ids {
		r := m.recs[id]
		if r.status != "" {
			continue
		}
		for _, p := range r.m.Provides {
			c := Capability{ID: p.ID, Version: p.Version, Provider: id, Available: false, Reason: "extension is not enabled", Entitlement: p.entitlementOf()}
			if err := m.reg.Offer(c); err != nil {
				r.status, r.detail = StatusConflict, Clean(err.Error(), 300, false)
				break
			}
		}
	}
	// Approvals, executable integrity and entitlements.
	for _, id := range m.order {
		r := m.recs[id]
		if r.status != "" {
			continue
		}
		r.status = StatusDisabled
		if !r.entry.Enabled {
			continue
		}
		granted := r.effective()
		if missing := granted.Missing(r.m.Permissions); len(missing) > 0 {
			r.status, r.detail = StatusNeedsApproval, "requests new permissions: "+strings.Join(missing, ", ")
			continue
		}
		sum, err := fileSHA256(r.execPath)
		if err != nil || (r.entry.ExecSHA256 != "" && sum != r.entry.ExecSHA256) {
			r.status, r.detail = StatusNeedsApproval, "the executable changed since it was approved"
			continue
		}
		r.execSHA = sum
		need := append([]string{}, r.m.Entitlement)
		for _, p := range r.m.Provides {
			if strings.HasPrefix(p.ID, "pro.") {
				need = append(need, p.ID)
			}
		}
		for _, c := range need {
			if c == "" {
				continue
			}
			d, err := m.o.Entitlements.Check(ctx, c)
			if err != nil || !d.Allowed {
				r.status, r.detail = StatusEntitlement, "capability "+c+" is not entitled"
				break
			}
		}
		if r.status == StatusDisabled {
			r.status = StatusEnabled
		}
	}
	// Dependency resolution to a fixed point: a provider's capabilities become
	// available only while it is active, so chains and cycles settle or fail.
	for changed := true; changed; {
		changed = false
		for _, id := range m.order {
			r := m.recs[id]
			if r.status != StatusEnabled {
				continue
			}
			var missing []string
			for _, req := range r.m.Requires {
				name, ver := req, 1
				if err := m.reg.Require(name, ver); err != nil {
					missing = append(missing, req)
				}
			}
			if len(missing) > 0 {
				r.status, r.detail = StatusUnsatisfied, "requires unavailable capabilities: "+strings.Join(missing, ", ")
				changed = true
				continue
			}
			for _, p := range r.m.Provides {
				if c, ok := m.reg.Get(p.ID); ok && !c.Available {
					m.reg.SetAvailable(p.ID, true, "")
					changed = true
				}
			}
		}
	}
	for _, id := range m.order {
		r := m.recs[id]
		if r.status == StatusDisabled && !r.entry.Enabled {
			continue
		}
		for _, p := range r.m.Provides {
			if r.status != StatusEnabled {
				reason := string(r.status)
				if r.detail != "" {
					reason = r.detail
				}
				m.reg.SetAvailable(p.ID, false, reason)
			}
		}
	}
}

// effective is what the extension may use right now: what the user approved,
// limited to what the current manifest still asks for. A manifest that stops
// requesting a permission loses it immediately.
func (r *record) effective() PermissionSet {
	granted, out := NewPermissionSet(r.entry.Granted...), PermissionSet{}
	for _, p := range r.m.Permissions {
		if granted[p] {
			out[p] = true
		}
	}
	return out
}

func (p Provided) entitlementOf() string {
	if strings.HasPrefix(p.ID, "pro.") {
		return p.ID
	}
	return ""
}

func (m *Manager) dataRoot() string { return m.o.DataDir }

// Warnings returns non-fatal discovery messages.
func (m *Manager) Warnings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.warnings...)
}

// Capabilities lists core and extension capabilities.
func (m *Manager) Capabilities() []Capability {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reg.List()
}

func (m *Manager) info(r *record) Info {
	granted := r.effective()
	i := Info{ID: r.m.ID, Name: Clean(r.m.Name, 60, false), Version: r.m.Version, API: r.m.API, Description: Clean(r.m.Description, 400, false), Author: Clean(r.m.Author, 80, false),
		Status: r.status, StatusDetail: r.detail, Directory: r.dir, Requires: r.m.Requires, Optional: r.m.Optional, Provides: r.m.Provides, Entitlement: r.m.Entitlement,
		Commands: r.m.Commands, Panels: r.m.Panels, Events: r.m.Events.Subscribe, Storage: r.m.Storage, Health: r.health, Permissions: []PermissionInfo{}}
	i.Health.EventsDropped = r.dropped
	for _, id := range r.m.SortedPermissions() {
		p, _ := LookupPermission(id)
		i.Permissions = append(i.Permissions, PermissionInfo{ID: id, Description: p.Description, Enforcement: p.Enforcement, Sensitive: p.Sensitive, Granted: granted.Has(id)})
	}
	i.NeedsApproval = granted.Missing(r.m.Permissions)
	for _, c := range r.m.Config {
		v := c.Default
		if cur, ok := r.entry.Config[c.Key]; ok {
			v = cur
		}
		i.Config = append(i.Config, ConfigInfo{ConfigOption: c, Value: v})
	}
	return i
}

// List returns every discovered extension, sorted by id.
func (m *Manager) List() []Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Info, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.info(m.recs[id]))
	}
	return out
}

// Inspect returns one extension.
func (m *Manager) Inspect(id string) (Info, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recs[id]
	if !ok {
		return Info{}, &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	}
	return m.info(r), nil
}

func (m *Manager) update(id string, fn func(*stateEntry)) error {
	st, _ := readState(m.o.StateFile)
	e := st.Extensions[id]
	fn(&e)
	st.Extensions[id] = e
	return writeState(m.o.StateFile, st)
}

// Enable approves and activates an extension. approve must be true when the
// requested permissions are not already granted; callers obtain that consent
// from the user (the CLI's --yes, the TUI's confirmation).
func (m *Manager) Enable(ctx context.Context, id string, approve bool) error {
	m.mu.Lock()
	r, ok := m.recs[id]
	if !ok {
		m.mu.Unlock()
		return &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	}
	switch r.status {
	case StatusInvalid, StatusIncompatible, StatusConflict:
		d := r.detail
		m.mu.Unlock()
		return fmt.Errorf("cannot enable: %s", d)
	}
	granted := r.effective()
	missing := granted.Missing(r.m.Permissions)
	sum, err := fileSHA256(r.execPath)
	changed := err != nil || (r.entry.ExecSHA256 != "" && sum != r.entry.ExecSHA256)
	if (len(missing) > 0 || changed) && !approve {
		m.mu.Unlock()
		if len(missing) == 0 {
			return &ApprovalRequiredError{Permissions: []string{"(executable changed)"}}
		}
		return &ApprovalRequiredError{Permissions: missing}
	}
	if err != nil {
		m.mu.Unlock()
		return err
	}
	perms := r.m.SortedPermissions()
	version := r.m.Version
	m.mu.Unlock()
	// Persist the approval first so the resolver sees a coherent enabled state,
	// then activate; failure rolls the enablement back.
	prev, _ := readState(m.o.StateFile)
	if err := m.update(id, func(e *stateEntry) { e.Enabled, e.Granted, e.ExecSHA256, e.Version = true, perms, sum, version }); err != nil {
		return err
	}
	if err := m.Discover(ctx); err != nil {
		return err
	}
	info, _ := m.Inspect(id)
	if info.Status != StatusEnabled {
		m.restore(id, prev)
		return fmt.Errorf("cannot enable: %s", firstNonEmpty(info.StatusDetail, string(info.Status)))
	}
	if _, err := m.invoke(ctx, id, "activate", map[string]any{"host_api": APIVersion, "host_version": HostVersion}); err != nil {
		m.restore(id, prev)
		_ = m.Discover(ctx)
		return fmt.Errorf("activation failed: %w", err)
	}
	m.mu.Lock()
	if r := m.recs[id]; r != nil {
		r.health.ConsecutiveFailures = 0
	}
	m.mu.Unlock()
	m.startDispatcher(id)
	return nil
}

func (m *Manager) restore(id string, prev stateFile) {
	old, had := prev.Extensions[id]
	_ = m.update(id, func(e *stateEntry) {
		if had {
			*e = old
		} else {
			*e = stateEntry{}
		}
	})
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// Disable stops an extension. Its data and granted permissions are kept.
func (m *Manager) Disable(ctx context.Context, id string) error {
	m.mu.Lock()
	r, ok := m.recs[id]
	active := ok && r.status == StatusEnabled
	m.mu.Unlock()
	if !ok {
		return &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	}
	m.stopDispatcher(id)
	if active {
		dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, _ = m.invoke(dctx, id, "deactivate", map[string]any{})
		cancel()
	}
	if err := m.update(id, func(e *stateEntry) { e.Enabled = false }); err != nil {
		return err
	}
	return m.Discover(ctx)
}

// Retry clears a tripped failure breaker by re-enabling with existing approval.
func (m *Manager) Retry(ctx context.Context, id string) error {
	m.mu.Lock()
	r, ok := m.recs[id]
	if ok {
		r.health.ConsecutiveFailures = 0
		if r.status == StatusFailed {
			r.status, r.detail = StatusEnabled, ""
		}
	}
	m.mu.Unlock()
	if !ok {
		return &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	}
	m.startDispatcher(id)
	return nil
}

// PurgeData deletes an extension's private storage. It is explicit and
// separate from Disable; the extension must not be enabled.
func (m *Manager) PurgeData(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.recs[id]; ok && (r.status == StatusEnabled || r.status == StatusFailed) {
		return fmt.Errorf("disable %s before purging its data", id)
	}
	s, err := OpenStorage(m.dataRoot(), id, 0)
	if err != nil {
		return err
	}
	return s.Purge()
}

// SetConfig validates and stores one setting.
func (m *Manager) SetConfig(id, key, value string) error {
	m.mu.Lock()
	r, ok := m.recs[id]
	if !ok {
		m.mu.Unlock()
		return &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	}
	var opt *ConfigOption
	for i := range r.m.Config {
		if r.m.Config[i].Key == key {
			opt = &r.m.Config[i]
		}
	}
	m.mu.Unlock()
	if opt == nil {
		return &ProtocolError{CodeNotFound, fmt.Sprintf("%s has no setting %q", id, key)}
	}
	v, err := opt.ParseConfigText(value)
	if err != nil {
		return &ProtocolError{CodeInvalid, fmt.Sprintf("%s: %v", key, err)}
	}
	if err := m.update(id, func(e *stateEntry) {
		if e.Config == nil {
			e.Config = map[string]any{}
		}
		e.Config[key] = v
	}); err != nil {
		return err
	}
	m.mu.Lock()
	if r.entry.Config == nil {
		r.entry.Config = map[string]any{}
	}
	r.entry.Config[key] = v
	m.mu.Unlock()
	return nil
}

func (m *Manager) session(r *record) *Session {
	cfg := map[string]any{}
	for _, c := range r.m.Config {
		cfg[c.Key] = c.Default
	}
	for k, v := range r.entry.Config {
		cfg[k] = v
	}
	return &Session{ID: r.m.ID, Granted: r.effective(), Repo: m.o.Repo, Storage: r.store, Config: cfg, Entitlement: r.m.Entitlement, Entitled: m.o.Entitlements}
}

// invoke runs one request against an active (or activating) extension and
// records health. It never panics the host and always reaps the process.
func (m *Manager) invoke(ctx context.Context, id, method string, params any) (json.RawMessage, error) {
	m.mu.Lock()
	r, ok := m.recs[id]
	if !ok {
		m.mu.Unlock()
		return nil, &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	}
	if r.status != StatusEnabled {
		st := r.status
		m.mu.Unlock()
		return nil, fmt.Errorf("extension %s is %s", id, st)
	}
	sess, dir, cmd, sem := m.session(r), r.dir, r.m.Command, r.sem
	m.mu.Unlock()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	res, err := Invoke(ctx, dir, cmd, id, method, params, m.o.Timeout, sess.Call)
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := m.recs[id]
	if cur == nil {
		return res.Data, err
	}
	cur.health.Invocations++
	cur.health.LastDurationMS = res.Duration.Milliseconds()
	if res.Stderr != "" {
		cur.health.Diagnostic = res.Stderr
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		var pe *ProtocolError
		// An extension reporting a clean business error is not a crash.
		if !errors.As(err, &pe) {
			cur.health.ConsecutiveFailures++
		}
		cur.health.LastError, cur.health.LastErrorAt = Clean(err.Error(), 300, false), now
		if cur.health.ConsecutiveFailures >= failureThreshold && cur.status == StatusEnabled {
			cur.status, cur.detail = StatusFailed, "stopped after repeated failures; re-enable to retry"
			m.o.Logger.Warn("extension failed", "extension", id)
		}
		return nil, err
	}
	cur.health.ConsecutiveFailures, cur.health.LastOK = 0, now
	return res.Data, nil
}

// RunCommand executes a declared command and returns sanitized host-rendered content.
func (m *Manager) RunCommand(ctx context.Context, id, command string, args map[string]string) (Content, error) {
	m.mu.Lock()
	r, ok := m.recs[id]
	var found bool
	var granted bool
	if ok {
		for _, c := range r.m.Commands {
			found = found || c.ID == command
		}
		granted = r.effective().Has("commands.register")
	}
	m.mu.Unlock()
	switch {
	case !ok:
		return Content{}, &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	case !found:
		return Content{}, &ProtocolError{CodeNotFound, fmt.Sprintf("%s has no command %q", id, command)}
	case !granted:
		return Content{}, &ProtocolError{CodePermissionDenied, "permission commands.register is not granted"}
	}
	if len(args) > 16 {
		return Content{}, &ProtocolError{CodeInvalid, "too many arguments"}
	}
	data, err := m.invoke(ctx, id, "command.run", map[string]any{"command": command, "args": args})
	if err != nil {
		return Content{}, err
	}
	return parseContent(data)
}

// RenderPanel asks an extension to render a declared panel for an entity.
func (m *Manager) RenderPanel(ctx context.Context, id, panel, entityID string) (Content, error) {
	m.mu.Lock()
	r, ok := m.recs[id]
	var found, granted bool
	if ok {
		for _, p := range r.m.Panels {
			found = found || p.ID == panel
		}
		granted = r.effective().Has("ui.register")
	}
	m.mu.Unlock()
	switch {
	case !ok:
		return Content{}, &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	case !found:
		return Content{}, &ProtocolError{CodeNotFound, fmt.Sprintf("%s has no panel %q", id, panel)}
	case !granted:
		return Content{}, &ProtocolError{CodePermissionDenied, "permission ui.register is not granted"}
	}
	data, err := m.invoke(ctx, id, "panel.render", map[string]any{"panel": panel, "entity_id": entityID})
	if err != nil {
		return Content{}, err
	}
	return parseContent(data)
}

func parseContent(data json.RawMessage) (Content, error) {
	var c Content
	if err := json.Unmarshal(data, &c); err != nil {
		return Content{}, fmt.Errorf("protocol violation: invalid content")
	}
	return SanitizeContent(c), nil
}

// CheckHealth asks the extension for its own health and records the result.
func (m *Manager) CheckHealth(ctx context.Context, id string) (Health, error) {
	data, err := m.invoke(ctx, id, "health", map[string]any{})
	if err == nil {
		var h struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &h) == nil && !h.OK {
			err = fmt.Errorf("unhealthy: %s", Clean(h.Message, 200, false))
		}
	}
	info, ierr := m.Inspect(id)
	if ierr != nil {
		return Health{}, ierr
	}
	return info.Health, err
}

// CommandRef is a command-palette contribution.
type CommandRef struct {
	Extension, ExtensionName, ID, Title, Description string
}

// Commands returns commands of active extensions that hold commands.register.
func (m *Manager) Commands() []CommandRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []CommandRef
	for _, id := range m.order {
		r := m.recs[id]
		if r.status != StatusEnabled || !r.effective().Has("commands.register") {
			continue
		}
		for _, c := range r.m.Commands {
			out = append(out, CommandRef{id, Clean(r.m.Name, 60, false), c.ID, c.Title, c.Description})
		}
	}
	return out
}

// PanelRef is a panel contribution.
type PanelRef struct{ Extension, ID, Title, Slot string }

// Panels returns panels in a slot from active extensions holding ui.register.
func (m *Manager) Panels(slot string) []PanelRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []PanelRef
	for _, id := range m.order {
		r := m.recs[id]
		if r.status != StatusEnabled || !r.effective().Has("ui.register") {
			continue
		}
		for _, p := range r.m.Panels {
			if p.Slot == slot {
				out = append(out, PanelRef{id, p.ID, p.Title, p.Slot})
			}
		}
	}
	return out
}

// Start activates previously enabled extensions and begins event delivery.
// Activation runs in the background; the caller (including a Bubble Tea
// program) is never blocked by extension code.
func (m *Manager) Start() {
	if m.o.Bus != nil && m.sub == nil {
		m.sub = m.o.Bus.Subscribe(256)
		m.wg.Add(1)
		go m.route()
	}
	m.mu.Lock()
	var ids []string
	for _, id := range m.order {
		if m.recs[id].status == StatusEnabled {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		id := id
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if _, err := m.invoke(m.ctx, id, "activate", map[string]any{"host_api": APIVersion, "host_version": HostVersion}); err != nil {
				m.o.Logger.Warn("extension activation failed", "extension", id)
				return
			}
			m.startDispatcher(id)
		}()
	}
}

// Close cancels delivery, kills in-flight extension processes and waits for
// every goroutine, so no child outlives the host.
func (m *Manager) Close() {
	m.stop()
	if m.sub != nil {
		m.sub.Close()
	}
	m.mu.Lock()
	for _, r := range m.recs {
		if r.cancel != nil {
			r.cancel()
		}
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) startDispatcher(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recs[id]
	if !ok || r.queue != nil || r.status != StatusEnabled || len(r.m.Events.Subscribe) == 0 || m.o.Bus == nil || m.ctx.Err() != nil {
		return
	}
	if !r.effective().Has("events.subscribe") {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	r.queue, r.cancel = make(chan Event, eventQueueSize), cancel
	q := r.queue
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-q:
				if _, err := m.invoke(ctx, id, "event.deliver", map[string]any{"event": e}); err == nil {
					m.mu.Lock()
					if cur := m.recs[id]; cur != nil {
						cur.health.EventsDelivered++
					}
					m.mu.Unlock()
				} else if ctx.Err() != nil {
					return
				}
				m.mu.Lock()
				cur := m.recs[id]
				stopped := cur == nil || cur.status != StatusEnabled
				m.mu.Unlock()
				if stopped {
					return
				}
			}
		}
	}()
}

func (m *Manager) stopDispatcher(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.recs[id]; ok && r.cancel != nil {
		r.cancel()
		r.cancel, r.queue = nil, nil
	}
}

// route copies bus events to per-extension queues. Events caused by
// extensions are never delivered to extensions, which prevents loops. A full
// queue drops the event and counts it: delivery is best-effort.
func (m *Manager) route() {
	defer m.wg.Done()
	for e := range m.sub.C {
		if IsExtensionSource(e.Source) {
			continue
		}
		m.mu.Lock()
		for _, id := range m.order {
			r := m.recs[id]
			if r.queue == nil || r.status != StatusEnabled {
				continue
			}
			want := false
			for _, t := range r.m.Events.Subscribe {
				want = want || t == e.Type
			}
			if info, ok := LookupEventType(e.Type); !want || !ok || !r.effective().Has(info.ReadPermission) {
				continue
			}
			select {
			case r.queue <- e:
			default:
				r.dropped++
			}
		}
		m.mu.Unlock()
	}
}

// PathsFor derives extension locations from the host's configuration and
// database paths: extensions live beside config.toml, their private data and
// the approval state beside the database. The data root is deliberately named
// differently from the program directory: the two may share a parent when the
// XDG config and data directories coincide, and purging data must never be able
// to touch a program. TIDECONTACTS_EXTENSIONS_DIR overrides
// the extension directory (development and tests).
func PathsFor(configFile, databaseFile string) (dir, dataDir, stateFile string) {
	dir = filepath.Join(filepath.Dir(configFile), "extensions")
	if v := os.Getenv("TIDECONTACTS_EXTENSIONS_DIR"); v != "" && filepath.IsAbs(v) {
		dir = v
	}
	base := filepath.Dir(databaseFile)
	return dir, filepath.Join(base, "extension-data"), filepath.Join(base, "extensions.json")
}
