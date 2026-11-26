package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/demo"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/mcpserver"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func IsMCPServe(args []string) bool {
	rest, _, _, err := globalFlags(args)
	return err == nil && len(rest) >= 2 && rest[0] == "mcp" && rest[1] == "serve"
}

func mcpError(err error) error {
	var p *mcpserver.Problem
	switch {
	case errors.As(err, &p):
		if p.Code == "permission_denied" {
			return fail(6, p.Code, p.Message)
		}
		return fail(3, p.Code, p.Message)
	case errors.Is(err, sql.ErrNoRows):
		return fail(4, "not_found", "MCP profile not found")
	case errors.Is(err, sqlite.ErrConflict):
		return fail(5, "conflict", "MCP profile already exists, its revision changed, or profile limit was reached")
	default:
		return err
	}
}

const mcpHelp = "mcp profiles|profile-set NAME|profile-disable NAME|capabilities NAME|diagnose NAME|config NAME CLIENT|audit|serve NAME"

func runMCP(ctx context.Context, args []string, demoMode bool, database string, in io.Reader, out io.Writer) error {
	if len(args) == 1 && slices.Contains([]string{"--help", "help"}, args[0]) {
		_, err := fmt.Fprintln(out, mcpHelp)
		return err
	}
	if len(args) == 0 {
		return usage(mcpHelp)
	}
	action, args := args[0], args[1:]
	var name, client string
	var p app.MCPProfile
	var before int64
	limit := 50
	switch action {
	case "serve", "diagnose", "capabilities", "profile-set", "profile-disable", "config":
		if len(args) == 0 || !mcpserver.ValidProfileName(args[0]) {
			return usage("specify a local MCP profile name (lowercase letters, digits, or hyphens)")
		}
		name, args = args[0], args[1:]
	case "profiles", "audit":
	default:
		return usage(mcpHelp)
	}
	f := flag.NewFlagSet("mcp "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	switch action {
	case "profile-set", "profile-disable":
		var scopes string
		var yes bool
		f.Int64Var(&p.Revision, "revision", 0, "expected revision for an existing profile")
		f.BoolVar(&yes, "yes", false, "confirm the exact profile grant")
		if action == "profile-set" {
			f.StringVar(&scopes, "scopes", "", "comma-separated scopes; none by default")
		}
		if err := f.Parse(args); err != nil || f.NArg() != 0 {
			return usage("invalid MCP profile options")
		}
		if !yes {
			return fail(6, "approval_required", "review the requested grants and repeat with --yes")
		}
		p.Name, p.Enabled, p.Scopes = name, action == "profile-set", []string{}
		if scopes != "" {
			p.Scopes = strings.Split(scopes, ",")
		}
		if err := mcpserver.ValidateProfile(&p); err != nil {
			return mcpError(err)
		}
		args = nil
	case "audit":
		f.Int64Var(&before, "before", 0, "exclusive descending sequence cursor")
		f.IntVar(&limit, "limit", 50, "page size, 1-100")
		if err := f.Parse(args); err != nil || f.NArg() != 0 || before < 0 || limit < 1 || limit > 100 {
			return usage("invalid audit cursor or limit (1-100)")
		}
		args = nil
	case "config":
		if len(args) != 1 || !slices.Contains([]string{"codex", "claude", "hermes"}, args[0]) {
			return usage("usage: mcp config PROFILE codex|claude|hermes")
		}
		client, args = args[0], nil
	}
	if len(args) != 0 {
		return usage(mcpHelp)
	}
	if action == "profile-disable" && p.Revision < 1 {
		return usage("profile-disable requires the current --revision")
	}
	paths, err := resolvePaths(demoMode, database)
	if err != nil {
		return err
	}
	s, err := sqlite.Open(ctx, paths.Database)
	if err != nil {
		return err
	}
	defer s.Close()
	var data any
	switch action {
	case "profiles":
		data, err = s.MCPProfiles(ctx)
	case "audit":
		data, err = s.MCPAuditPage(ctx, before, limit)
	case "profile-set", "profile-disable":
		data, err = s.SaveMCPProfile(ctx, p)
	default:
		var manager *ext.Manager
		if p, err := s.MCPProfile(ctx, name); err == nil && p.Enabled && mcpserver.WantsProfessional(p.Scopes) {
			manager = newManager(paths, s)
			defer manager.Close()
		}
		server, buildErr := mcpserver.New(ctx, s, s, name, manager)
		if buildErr != nil {
			return mcpError(buildErr)
		}
		switch action {
		case "serve":
			pipe, ok := in.(io.ReadCloser)
			if !ok {
				return fmt.Errorf("MCP stdio requires a closable input stream")
			}
			if demoMode {
				if err := demo.Seed(ctx, s); err != nil {
					return err
				}
			}
			return server.Run(ctx, pipe, out)
		case "capabilities":
			data = server.Info()
		case "diagnose":
			data, err = server.Diagnose(ctx)
		case "config":
			data, err = mcpClientConfig(client, name, paths.Database, demoMode, database != "")
		}
	}
	if err != nil {
		return mcpError(err)
	}
	return json.NewEncoder(out).Encode(envelope{Version: 1, Data: data, Meta: &metadata{Resource: "mcp", Action: action}})
}

func mcpClientConfig(client, profile, database string, demoMode, explicitDB bool) (any, error) {
	command, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{}
	if demoMode {
		args = append(args, "--demo")
	} else if explicitDB {
		args = append(args, "--database="+database)
	}
	args = append(args, "mcp", "serve", profile)
	var snippet string
	quotedCommand, _ := json.Marshal(command)
	quotedArgs, _ := json.Marshal(args)
	switch client {
	case "codex":
		snippet = fmt.Sprintf("[mcp_servers.tidecontacts]\ncommand = %s\nargs = %s\n", quotedCommand, quotedArgs)
	case "hermes":
		snippet = fmt.Sprintf("mcp_servers:\n  tidecontacts:\n    command: %s\n    args: %s\n", quotedCommand, quotedArgs)
	case "claude":
		b, err := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"tidecontacts": map[string]any{
			"type": "stdio", "command": command, "args": args,
		}}}, "", "  ")
		if err != nil {
			return nil, err
		}
		snippet = string(b) + "\n"
	}
	return map[string]any{"client": client, "command": command, "args": args, "snippet": snippet}, nil
}

func mcpSchemaInfo() any {
	return map[string]any{"resource": "mcp", "transport": "stdio", "usage": mcpHelp, "scopes": mcpserver.Scopes()}
}
