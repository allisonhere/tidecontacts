package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// ExtensionsHelp documents the extension commands.
const ExtensionsHelp = `
  extensions             list|inspect|permissions|commands|config|health|enable|disable|run|purge-data
  capabilities           list
`

var extensionActions = map[string]bool{"list": true, "inspect": true, "permissions": true, "commands": true, "config": true, "health": true, "enable": true, "disable": true, "run": true, "purge-data": true}

// extCommand is one declared command with its availability.
type extCommand struct {
	Extension string `json:"extension"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Available bool   `json:"available"`
}

func extError(err error) error {
	var pe *ext.ProtocolError
	var ar *ext.ApprovalRequiredError
	switch {
	case errors.As(err, &ar):
		return fail(6, "approval_required", err.Error()+"; review with 'extensions permissions' and repeat with --yes")
	case errors.As(err, &pe):
		switch pe.Code {
		case ext.CodeNotFound:
			return fail(4, "not_found", pe.Message)
		case ext.CodeConflict:
			return fail(5, "conflict", pe.Message)
		case ext.CodePermissionDenied, ext.CodeEntitlement:
			return fail(6, pe.Code, pe.Message)
		case ext.CodeInvalid:
			return invalid(pe.Message)
		}
	}
	return err
}

// runExtensions serves the extensions and capabilities resources. It uses the
// same ext.Manager, permission checks and validation as the TUI.
func runExtensions(ctx context.Context, resource, action, id, command string, o options, input []byte, paths config.Paths, out io.Writer) error {
	if resource == "capabilities" {
		if action != "list" || id != "" {
			return usage("usage: capabilities list")
		}
	} else if !extensionActions[action] {
		return usage("unknown extensions action " + action)
	}
	needsID := map[string]bool{"inspect": true, "permissions": true, "health": true, "enable": true, "disable": true, "run": true, "purge-data": true, "config": true}
	if resource == "extensions" && needsID[action] && id == "" {
		return usage("extensions " + action + " requires an extension id")
	}
	if action == "run" && command == "" {
		return usage("usage: extensions run ID COMMAND")
	}
	if action == "purge-data" && !o.yes {
		return usage("purge-data deletes the extension's private data; repeat with --yes")
	}
	dir, data, state := ext.PathsFor(paths.Config, paths.Database)
	opts := ext.Options{Dir: dir, DataDir: data, StateFile: state}
	if action == "run" {
		s, err := sqlite.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		var repo app.CRM = s
		opts.Repo = repo
	}
	m := ext.New(opts)
	defer m.Close()
	if err := m.Discover(ctx); err != nil {
		return err
	}
	var result any
	var err error
	switch {
	case resource == "capabilities":
		result = m.Capabilities()
	case action == "list":
		result = m.List()
	case action == "inspect":
		result, err = m.Inspect(id)
	case action == "permissions":
		var i ext.Info
		if i, err = m.Inspect(id); err == nil {
			result = i.Permissions
		}
	case action == "commands":
		cmds := []extCommand{}
		for _, i := range m.List() {
			if id != "" && i.ID != id {
				continue
			}
			for _, c := range i.Commands {
				cmds = append(cmds, extCommand{i.ID, c.ID, c.Title, i.Status == ext.StatusEnabled})
			}
		}
		sort.Slice(cmds, func(a, b int) bool {
			return cmds[a].Extension+"/"+cmds[a].ID < cmds[b].Extension+"/"+cmds[b].ID
		})
		result = cmds
	case action == "config":
		if o.set != "" {
			k, v, ok := cutPair(o.set)
			if !ok {
				return usage("--set expects KEY=VALUE")
			}
			if err = m.SetConfig(id, k, v); err != nil {
				return extError(err)
			}
		}
		var i ext.Info
		if i, err = m.Inspect(id); err == nil {
			result = i.Config
		}
	case action == "health":
		var h ext.Health
		h, err = m.CheckHealth(ctx, id)
		result = h
	case action == "enable":
		if err = m.Enable(ctx, id, o.yes); err == nil {
			result, err = m.Inspect(id)
		}
	case action == "disable":
		if err = m.Disable(ctx, id); err == nil {
			result, err = m.Inspect(id)
		}
	case action == "purge-data":
		if err = m.PurgeData(ctx, id); err == nil {
			result = map[string]any{"id": id, "purged": true}
		}
	case action == "run":
		args := map[string]string{}
		if len(input) > 0 {
			if e := json.Unmarshal(input, &args); e != nil {
				return invalid("--input must be a JSON object of string arguments")
			}
		}
		result, err = m.RunCommand(ctx, id, command, args)
	}
	if err != nil {
		return extError(err)
	}
	resp := envelope{Version: 1, Data: result, Meta: &metadata{Resource: resource, Action: action}}
	switch v := result.(type) {
	case []ext.Info:
		resp.Meta.Count = len(v)
	case []ext.Capability:
		resp.Meta.Count = len(v)
	case []extCommand:
		resp.Meta.Count = len(v)
	case []ext.PermissionInfo:
		resp.Meta.Count = len(v)
	case []ext.ConfigInfo:
		resp.Meta.Count = len(v)
	default:
		resp.Meta.Count = 1
	}
	if w := m.Warnings(); len(w) > 0 {
		resp.Warnings = w
	}
	return json.NewEncoder(out).Encode(resp)
}

func cutPair(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], i > 0
		}
	}
	return "", "", false
}
