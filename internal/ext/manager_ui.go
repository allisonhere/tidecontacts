package ext

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// WorkspaceRef is a workspace contributed by an enabled extension.
type WorkspaceRef struct{ Extension, ExtensionName, ID, Title string }

// Workspaces lists workspaces of enabled extensions holding ui.register. With
// none installed or enabled the result is empty and the menu is unchanged.
func (m *Manager) Workspaces() []WorkspaceRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []WorkspaceRef
	for _, id := range m.order {
		r := m.recs[id]
		if r.status != StatusEnabled || !r.effective().Has("ui.register") {
			continue
		}
		for _, w := range r.m.Workspaces {
			out = append(out, WorkspaceRef{id, Clean(r.m.Name, 60, false), w.ID, Clean(w.Title, 24, false)})
		}
	}
	return out
}

// require checks that id is enabled and holds perm (default deny).
func (m *Manager) require(id, perm string) *ProtocolError {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.recs[id]
	switch {
	case !ok:
		return &ProtocolError{CodeNotFound, fmt.Sprintf("no extension %q", id)}
	case !r.effective().Has(perm):
		return &ProtocolError{CodePermissionDenied, "permission " + perm + " is not granted"}
	}
	return nil
}

func (m *Manager) hasWorkspace(id, ws string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.recs[id]; ok {
		for _, w := range r.m.Workspaces {
			if w.ID == ws {
				return true
			}
		}
	}
	return false
}

func (m *Manager) call(ctx context.Context, id, method, perm string, params, out any) error {
	if e := m.require(id, perm); e != nil {
		return e
	}
	data, err := m.invoke(ctx, id, method, params)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("protocol violation: invalid %s response", method)
	}
	return nil
}

// RenderView asks an extension to describe one view of one of its workspaces.
func (m *Manager) RenderView(ctx context.Context, id string, req ViewRequest) (View, error) {
	if !m.hasWorkspace(id, req.Workspace) {
		return View{}, &ProtocolError{CodeNotFound, fmt.Sprintf("%s has no workspace %q", id, req.Workspace)}
	}
	var v View
	if err := m.call(ctx, id, MethodViewRender, "ui.register", req, &v); err != nil {
		return View{}, err
	}
	return SanitizeView(v), nil
}

// RunAction runs a view action.
func (m *Manager) RunAction(ctx context.Context, id string, req ActionRequest) (ActionResult, error) {
	if !m.hasWorkspace(id, req.Workspace) {
		return ActionResult{}, &ProtocolError{CodeNotFound, fmt.Sprintf("%s has no workspace %q", id, req.Workspace)}
	}
	var r ActionResult
	if err := m.call(ctx, id, MethodActionRun, "ui.register", req, &r); err != nil {
		return ActionResult{}, err
	}
	return SanitizeResult(r), nil
}

// GetForm fetches a form definition.
func (m *Manager) GetForm(ctx context.Context, id string, req FormRequest) (Form, error) {
	var f Form
	if err := m.call(ctx, id, MethodFormGet, "ui.register", req, &f); err != nil {
		return Form{}, err
	}
	return SanitizeForm(f), nil
}

// SubmitForm delivers answers; the extension validates and may answer with field errors.
func (m *Manager) SubmitForm(ctx context.Context, id string, req FormSubmit) (ActionResult, error) {
	var r ActionResult
	if err := m.call(ctx, id, MethodFormSubmit, "ui.register", req, &r); err != nil {
		return ActionResult{}, err
	}
	return SanitizeResult(r), nil
}

// RunCommandResult runs a palette command and returns the full result, which
// may open a form or a workspace as well as show text.
func (m *Manager) RunCommandResult(ctx context.Context, id, command string, args map[string]string) (ActionResult, error) {
	var found bool
	m.mu.Lock()
	if r, ok := m.recs[id]; ok {
		for _, c := range r.m.Commands {
			found = found || c.ID == command
		}
	}
	m.mu.Unlock()
	if !found {
		return ActionResult{}, &ProtocolError{CodeNotFound, fmt.Sprintf("%s has no command %q", id, command)}
	}
	var r ActionResult
	if err := m.call(ctx, id, "command.run", "commands.register", map[string]any{"command": command, "args": args}, &r); err != nil {
		return ActionResult{}, err
	}
	return SanitizeResult(r), nil
}

// SignalRef is a contributed signal with its source.
type SignalRef struct {
	Extension string `json:"extension"`
	Signal
}

// Signals gathers signals from every enabled extension that provides them.
// A failing extension contributes nothing and never blocks the others.
func (m *Manager) Signals(ctx context.Context) []SignalRef {
	m.mu.Lock()
	var ids []string
	for _, id := range m.order {
		r := m.recs[id]
		if r.status == StatusEnabled && r.m.Signals && r.effective().Has("signals.provide") {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	var out []SignalRef
	for _, id := range ids {
		var list []Signal
		if err := m.call(ctx, id, MethodSignals, "signals.provide", map[string]any{"limit": maxSignals}, &list); err != nil {
			continue
		}
		for _, s := range SanitizeSignals(list) {
			out = append(out, SignalRef{id, s})
		}
	}
	return out
}

// CLIRef is a CLI resource contributed by an enabled extension.
type CLIRef struct {
	Extension   string   `json:"extension"`
	Resource    string   `json:"resource"`
	Description string   `json:"description,omitempty"`
	Actions     []string `json:"actions,omitempty"`
}

// CLIResources lists resources of enabled extensions holding cli.register.
func (m *Manager) CLIResources() []CLIRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []CLIRef
	for _, id := range m.order {
		r := m.recs[id]
		if r.status != StatusEnabled || !r.effective().Has("cli.register") {
			continue
		}
		for _, c := range r.m.CLI {
			out = append(out, CLIRef{id, c.Resource, Clean(c.Description, 120, false), c.Actions})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	return out
}

// CLIRequest is a command-line invocation routed to an extension. Argv is the
// raw argument list after the resource name; the extension parses its own flags.
type CLIRequest struct {
	Resource string   `json:"resource"`
	Argv     []string `json:"argv"`
	Stdin    string   `json:"stdin,omitempty"` // only when --input - was given (≤ 900 KiB)
}

// CLIResponse is the extension's structured answer; the host builds the envelope.
type CLIResponse struct {
	Data      json.RawMessage `json:"data"`
	Count     int             `json:"count,omitempty"`
	ETag      string          `json:"etag,omitempty"`
	Truncated bool            `json:"possibly_truncated,omitempty"`
	Warnings  []string        `json:"warnings,omitempty"`
}

// RunCLI serves a contributed CLI resource.
func (m *Manager) RunCLI(ctx context.Context, req CLIRequest) (CLIResponse, error) {
	id := ""
	for _, c := range m.CLIResources() {
		if c.Resource == req.Resource {
			id = c.Extension
		}
	}
	if id == "" {
		return CLIResponse{}, &ProtocolError{CodeNotFound, fmt.Sprintf("no enabled extension provides %q", req.Resource)}
	}
	var r CLIResponse
	if err := m.call(ctx, id, MethodCLIRun, "cli.register", req, &r); err != nil {
		return CLIResponse{}, err
	}
	for i, w := range r.Warnings {
		r.Warnings[i] = Clean(w, 200, false)
	}
	r.ETag = Clean(r.ETag, 128, false)
	return r, nil
}

// CLISchema asks the owning extension for a JSON Schema of its resource.
func (m *Manager) CLISchema(ctx context.Context, resource string) (json.RawMessage, error) {
	for _, c := range m.CLIResources() {
		if c.Resource == resource {
			var raw json.RawMessage
			if err := m.call(ctx, c.Extension, MethodCLISchema, "cli.register", map[string]any{"resource": resource}, &raw); err != nil {
				return nil, err
			}
			return raw, nil
		}
	}
	return nil, &ProtocolError{CodeNotFound, fmt.Sprintf("no enabled extension provides %q", resource)}
}
