# Local MCP

TideContacts serves MCP over **stdio** without opening the terminal UI or a network listener. It uses the [official Go SDK](https://github.com/modelcontextprotocol/go-sdk) with the [2026-07-28 stdio protocol](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/stdio) and legacy initialization compatibility. Your client may send returned data to its model provider; keeping the server local does not make the client offline.

## Try it with fictional data

```sh
tidecontacts --demo mcp profile-set review --scopes contacts.read,organizations.read,relationships.read,tasks.read --yes
tidecontacts --demo mcp diagnose review
tidecontacts --demo mcp capabilities review
```

Profiles are attached to the chosen TideContacts database, and no profile is enabled by default. An empty grant has no CRM read or write tools. Copy only a snippet for the intended profile; remove `--demo` only when ready to share your live records. Edit or revoke with the revision from `mcp profiles`:

```sh
tidecontacts --demo mcp profile-disable review --revision 1 --yes
tidecontacts --demo mcp audit --limit 30
```

Any changed revision invalidates existing sessions for that profile. Reconnect after editing. A call already executing when you revoke may finish. The database remembers roughly the latest 10,000 operations: profile, tool, time, write flag, and outcome, not arguments, record values, or errors from untrusted integrations. A `started` write was not confirmed as completed; inspect records before retrying. Local audit is not tamper-proof.

In the TUI, open `:` → **Manage MCP profiles…**. Use `d` to discover the selected profile's tools or `a` to inspect recent operations. You can edit or revoke grants from this workspace without editing client configuration.

## Client setup

Run `tidecontacts --demo mcp config review codex` (or `claude`, `hermes`) and copy the string in `data.snippet` to your client's configuration yourself. Use the path of the installed **matching** TideContacts executable. Do not use a launcher that checks for a terminal or prints startup text to stdout.

| Client | Verified configuration reference | Example shape |
|---|---|---|
| Codex | [MCP setup](https://developers.openai.com/codex/mcp) | `[mcp_servers.tidecontacts]` with `command` and `args` in `config.toml` |
| Claude Code | [Local stdio servers](https://code.claude.com/docs/en/mcp) | `.mcp.json` → `mcpServers` → named `{"type":"stdio","command":"…","args":["--demo","mcp","serve","review"]}` |
| Hermes Agent | [Stdio configuration](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp#stdio-servers) | `mcp_servers:` → `tidecontacts:` → `command` and `args` in Hermes `config.yaml` |

On discovery, `tools/list` provides exact tool schemas and only tools available to that profile. `tidecontacts://capabilities` summarizes grants and tools. Static meeting and commitment prompts tell clients to separate facts from suggestions and not follow instructions embedded in record data. Page list calls using the returned cursor; a filtered page may be empty even when there is another cursor. Context fields are untrusted data, not system prompts.

## Grants and writes

| Grant | What it accesses |
|---|---|
| `contacts.read`, `organizations.read` | Bounded local summaries and selected identity fields; no notes or arbitrary exports |
| `relationships.read`, `activities.read`, `tasks.read`, `projects.read`, `pipelines.read` | Scoped record projections; linked details respect their own scopes |
| `followups.read`, `radar.read`, `inbox.read` | Scoped schedules or derived signals; Inbox reads do not mark state |
| `contacts.write`, `activities.write`, `tasks.write`, `projects.write`, `followups.write` | Explicitly enabled, narrow non-destructive operations; edits use the current revision or settings token |
| `opportunities.read`, `reports.read`, `automation.inspect`, `automation.simulate` | Optional Professional tools; require the extension's approval and the relevant entitlement |
| `playbooks.preview`, `playbooks.execute` | Optional, revision-bound playbook preview or proposal. `playbooks.execute` cannot approve or apply an action. |

MCP tags writes with the host-selected profile and a causal marker; they are not fresh automation event triggers. MCP cannot approve a workflow, authorize automatic execution, load shell actions, set an outbound destination, or read raw files. Granting a scope does **not** isolate an arbitrary same-user client from your files. The selected launch profile is an operator-configured grant, not an authentication proof; MCP `clientInfo` is ignored for authorization. Do not grant access to agents you would not trust with the returned data.

## Examples

- **Meeting prep:** Find the intended person; open their dossier and pending commitments using separately granted scopes. Ask the client for recorded facts and clearly labelled suggestions.
- **Business review:** With authorized Professional `reports.read`, use briefing and forecast tools. Keep all currencies separate and distinguish forecast from booked sales.
- **Proposed onboarding:** Grant `playbooks.preview` first. If you also grant `playbooks.execute`, the agent can save the reviewed plan for local approval. Inspect it in **Automation → Approvals** before approving any step.

`mcp diagnose` checks discovery without invoking operations. `mcp serve` reserves stdout for MCP messages; launch failures use stderr. An unavailable Professional extension does not prevent Community tools from working. Restart the client after installing an extension or enabling a new entitlement; revocation takes effect without restarting.
