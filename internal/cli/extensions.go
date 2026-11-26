package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/demo"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// ExtensionsHelp documents the extension commands.
const ExtensionsHelp = `
  extensions             list|inspect|permissions|commands|config|health|enable|disable|run|purge-data|prune-events
  capabilities           list
`

var extensionActions = map[string]bool{"list": true, "inspect": true, "permissions": true, "commands": true, "config": true, "health": true, "enable": true, "disable": true, "run": true, "purge-data": true, "prune-events": true}

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
	if action == "prune-events" {
		if id != "" || !o.yes {
			return usage("use extensions prune-events --yes")
		}
		s, err := sqlite.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		n, err := s.PruneEvents(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(envelope{Version: 1, Data: map[string]int64{"pruned": n}, Meta: &metadata{Resource: resource, Action: action, Count: int(n)}})
	}
	var repo app.CRM
	if action == "run" {
		s, err := sqlite.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		repo = s
	}
	m := newManager(paths, repo)
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

// globalFlags extracts the flags the host itself owns (--database, --demo) from
// an extension resource's argument list, leaving the rest for the extension.
func globalFlags(args []string) (rest []string, demoMode bool, database string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--demo":
			demoMode = true
		case a == "--database":
			if i+1 >= len(args) {
				return nil, false, "", usage("missing value for --database")
			}
			i++
			database = args[i]
		case strings.HasPrefix(a, "--database="):
			database = strings.TrimPrefix(a, "--database=")
		default:
			rest = append(rest, a)
		}
	}
	return rest, demoMode, database, nil
}

func newManager(paths config.Paths, repo app.CRM) *ext.Manager {
	dir, data, state := ext.PathsFor(paths.Config, paths.Database)
	prov, notice := ext.ProviderFromEnv()
	return ext.New(ext.Options{Dir: dir, DataDir: data, StateFile: state, Repo: repo, Entitlements: prov, Notice: notice})
}

func resolvePaths(demoMode bool, database string) (config.Paths, error) {
	paths, err := config.Resolve()
	if err != nil {
		return paths, err
	}
	if database != "" {
		if demoMode {
			return paths, usage("--database and --demo cannot be combined")
		}
		if !filepath.IsAbs(database) {
			return paths, usage("--database must be absolute")
		}
		paths.Database = database
	}
	if demoMode {
		paths = demo.Paths(paths)
	}
	return paths, nil
}

// extensionExit maps an extension's error code to the documented exit codes.
func extensionExit(err error) error {
	var pe *ext.ProtocolError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "usage":
			return usage(pe.Message)
		case ext.CodeInvalid:
			return invalid(pe.Message)
		case ext.CodeNotFound, ext.CodeConflict, ext.CodePermissionDenied, ext.CodeEntitlement:
			return extError(err)
		}
		return fail(1, ext.Clean(pe.Code, 40, false), pe.Message)
	}
	return extError(err)
}

const maxInlineStdin = 900 << 10

// runExtensionResource routes a top-level resource contributed by an enabled
// extension. It reports handled=false when no extension provides the resource,
// so unknown resources keep producing the ordinary usage error. Business rules
// live in the extension; this only carries arguments and shapes the envelope.
func runExtensionResource(ctx context.Context, args []string, in io.Reader, out io.Writer, demoMode bool, database string) (handled bool, err error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") || resources[args[0]] {
		return false, nil
	}
	paths, err := resolvePaths(demoMode, database)
	if err != nil {
		return true, err
	}
	// Discovery first, without opening the database: most typos end here.
	probe := newManager(paths, nil)
	defer probe.Close()
	if err := probe.Discover(ctx); err != nil {
		return false, nil
	}
	found := false
	for _, c := range probe.CLIResources() {
		found = found || c.Resource == args[0]
	}
	if !found {
		return false, nil
	}
	s, err := sqlite.Open(ctx, paths.Database)
	if err != nil {
		return true, err
	}
	defer func() { _ = s.Close() }()
	if demoMode {
		if err := demo.Seed(ctx, s); err != nil {
			return true, err
		}
	}
	var repo app.CRM = s
	m := newManager(paths, repo)
	defer m.Close()
	if err := m.Discover(ctx); err != nil {
		return true, err
	}
	req := ext.CLIRequest{Resource: args[0], Argv: args[1:]}
	for i, a := range req.Argv {
		if (a == "--input" && i+1 < len(req.Argv) && req.Argv[i+1] == "-") || a == "--input=-" {
			data, rerr := io.ReadAll(io.LimitReader(in, maxInlineStdin+1))
			if rerr != nil {
				return true, rerr
			}
			if len(data) > maxInlineStdin {
				return true, invalid("input from stdin exceeds 900 KiB; pass a file path with --input instead")
			}
			req.Stdin = string(data)
		}
	}
	resp, err := m.RunCLI(ctx, req)
	if err != nil {
		return true, extensionExit(err)
	}
	action := ""
	if len(req.Argv) > 0 {
		action = req.Argv[0]
	}
	if resp.Output != "" {
		raw := false
		for _, a := range req.Argv {
			switch a {
			case "--raw", "--raw=true":
				raw = true
			case "--raw=false":
				raw = false
			}
		}
		if !raw {
			return true, fail(1, "runtime", "extension returned raw output without --raw")
		}
		_, err := io.WriteString(out, resp.Output)
		return true, err
	}
	data := resp.Data
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	env := struct {
		Version  int             `json:"schema_version"`
		Data     json.RawMessage `json:"data"`
		ETag     string          `json:"etag,omitempty"`
		Meta     *metadata       `json:"meta"`
		Warnings []string        `json:"warnings,omitempty"`
	}{1, data, resp.ETag, &metadata{Resource: req.Resource, Action: action, Count: resp.Count, PossiblyTruncated: resp.Truncated}, append(m.Warnings(), resp.Warnings...)}
	return true, json.NewEncoder(out).Encode(env)
}

// extensionSchema serves `schema RESOURCE` for extension resources.
func extensionSchema(ctx context.Context, name string, demoMode bool, database string) (json.RawMessage, bool, error) {
	paths, err := resolvePaths(demoMode, database)
	if err != nil {
		return nil, false, err
	}
	m := newManager(paths, nil)
	defer m.Close()
	if err := m.Discover(ctx); err != nil {
		return nil, false, nil
	}
	for _, c := range m.CLIResources() {
		if c.Resource == name {
			raw, err := m.CLISchema(ctx, name)
			return raw, true, err
		}
	}
	return nil, false, nil
}
