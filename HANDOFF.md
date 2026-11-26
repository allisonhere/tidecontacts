# TideContacts handoff

Use [STATUS.md](STATUS.md) for the current feature inventory and [README.md](README.md) for setup. Earlier handoff entries below are historical.

## Phase 3D: Business Automation Studio host

Branch `feat/phase3d-automation-host`; pair with the private `feat/phase3d`
branch. Host **0.6.0**, wire protocol **v1**, Community schema **5**. The
Community app can be used without either installing or enabling Professional.

- `internal/storage/sqlite/eventlog.go`: transactional outbox, per-topic
  checkpoints, retry, dead letters, replay and explicitly confirmed cleanup.
- `internal/ext/host_eventlog.go`: session-owned consumer and current topic
  read grants. Durable envelope schema 2; legacy transient push is unchanged.
- `internal/ext/host_rule_records.go`: scoped permissions, note-free pages,
  tag revision checks and causal metadata on supported CRM writes.
- `internal/ext/ui.go` and `internal/tui/extview.go`: bounded declarative
  editor state and keyboard drill-down; no Professional rules or runner in
  Community. [Extension protocol](docs/extensions/specification.md#11-durable-event-journal-host-050-api-v1).

Relevant extension, SQLite, CLI and TUI race tests use only isolated fixtures.
The private checkout has paired CLI/PTY tests, measured benchmarks and the
Phase 4A application API handoff. Licensing is unchanged; nothing is authorized
for distribution by this work.

## Phase 3C: Command Center host

Branch `feat/phase3c-command-center`, stacked on Phase 3B; not merged or published. Extension host **0.4.0**, wire protocol **v1**, Community schema **4**. The Professional implementation, its demo, captures and performance record belong to the separate private checkout.

- `internal/viz`: bounded terminal-cell chart geometry with Unicode and reduced glyphs; `internal/tui/extpanels.go` assigns theme tones, column layouts and keyboard selection.
- `internal/ext/ui_panels.go`, `internal/tui/extview.go`: panel documents, actions, forms and drill-down. Esc restores context across extension workspaces or Community record links. Resizing across the compact breakpoint cancels and replaces the old render.
- `internal/ext/host_attention.go`: permission-checked reuse of the People Inbox, contact engagement and task completion. Do not add a competing reminder store.
- `internal/cli/extensions.go`: versioned extension CLI with explicit opt-in raw output. The host allows 16 CLI resources and a matching manifest must advertise host 0.4.0. See [extension specification §10](docs/extensions/specification.md#10-panel-views-and-shared-attention-host-040-api-v1).

Verify host panels with the `TestPanels`, `TestWorkspace` and extension CLI tests, including three terminal goldens. The private checkout has the paired CLI/PTY smoke script. No license or contributor clearance was changed.

## Phase 3B: host contributions for a Professional extension (November 25, 2026)

Branch `feat/phase3b-host-extension-points`, stacked on the 3A branch; not merged or pushed. Adds, in separate commits: UI documents (`internal/ext/ui.go`), manifest contributions, new permissions (`cli.register`, `signals.provide`, `events.publish`, `storage.sql`, `projects.write` enforced), host calls (`records.resolve`, `entitlement.check`, `events.publish`, `projects.save`), per-capability and development entitlements, CLI resource routing (`internal/cli/extensions.go`), the TUI workspace (`internal/tui/extview.go`) and Inbox signals (`internal/tui/extensions.go`). Spec: `docs/extensions/specification.md` §9; decisions ADR-9 to ADR-11; readiness analysis and its resolution: `docs/extensions/phase3b-readiness.md`. Professional business logic lives only in the private `tidecontacts-pro` repository.

Verify: `make check`, `make smoke-extensions`, `make smoke`. Seven Phase 3A commits (`git log origin/main..`) still carry a Claude `Co-Authored-By` trailer that the user asked not to add; strip with an interactive rebase before pushing if wanted.

## Phase 3A: extension architecture (November 25, 2026)

Branch `feat/phase3a-extension-architecture`; not merged, no release. Licensing is unchanged and still pending ([COMMERCIAL-LICENSING.md](COMMERCIAL-LICENSING.md), [checklist](docs/licensing-checklist.md)); [docs/extensions/licensing-boundary.md](docs/extensions/licensing-boundary.md) lists the open questions for counsel. Nothing commercial was implemented.

Code map:

- `internal/ext`: `manifest.go` (strict TOML + validation), `permissions.go`, `capability.go` (registry), `entitlement.go` (interface + Community provider), `events.go` (typed events, bounded bus), `host.go` (`Session`: permission-checked host API, the only door extensions get), `process.go` (+`process_unix/windows.go`: subprocess runner, limits, group kill), `storage.go`, `state.go`, `manager.go` (discovery, approval, lifecycle, health, event routing), `sanitize.go`.
- `internal/ext/observe`: wraps `*sqlite.Store`; publishes events after successful writes only. `cmd/tidecontacts/main.go` wires store → observe → manager and closes the manager before the store.
- `internal/cli/extensions.go`: `extensions …`, `capabilities list`, exit code 6.
- `internal/tui/extensions.go`: management modal, palette entries (`paletteMatches`), dossier panel rows. Only hooks in existing files: `Update` (first line), `dispatch`, `overlayPanel`, `loadDossier`, palette matching.
- `examples/extensions/relationship-digest`: demonstration extension. `make extension-demo`, `make smoke-extensions`.
- Docs: `docs/extensions/*`, `docs/adr/phase3a-extensions.md`.

Behavior to remember: schema is unchanged (4); no extension → no process, no files. Extension programs live in `<config>/tidecontacts/extensions/<id>/`, approvals in `<data>/tidecontacts/extensions.json`, private data in `<data>/tidecontacts/extension-data/<id>/` (deliberately a different name than the program directory). `TIDECONTACTS_EXTENSIONS_DIR` overrides the program directory.

Remaining (Phase 3B candidates): optional OS sandbox launcher and CPU/memory limits; durable event outbox and events for folder/Google refresh and TideMail sync; `project.status_changed`, `followup.due`, `reminder.due`; write permissions for relationships/projects/pipelines; more UI slots (navigation, dashboard, settings, search providers) once the host renders them; long-lived extension mode if call latency matters; migration runner for extension data; extension install/verify/update flow and signing; a local API server reusing `ext.Session`; counsel's answers on the licensing boundary. The existing `internal/tui` suite takes ~100 s on `main` too (pre-existing).

## Delivered

TideContacts has eight CRM workspaces, the TideMail activity connector, a shared TideMail contacts plugin, a JSON scripting CLI, read-only aerc/NeoMutt recipient output, and the import modal. The merged import sources are CSV, vCard, CRM JSON, Mutt/NeoMutt aliases and khard/vdirsyncer folders. Enabled sources automatically sort above the nine dimmed planned providers.

Connected folders keep stable contact identities by UID, refresh at startup or explicitly, and refresh through TideMail’s contacts.list/search path while TideContacts is stopped. Complete scans apply atomically; failed scans preserve the cache. Directory contacts are read-only at the storage boundary. Disconnect hides the records and retains mappings for reconnect. Folder connections read local vCards. The working tree also includes a direct Google Contacts connector. [Full behavior](docs/vcard-directories.md).

## Code map

- `internal/transfer/{transfer,mutt,directory}.go`: contact file parsing, alias parsing and complete directory snapshots.
- `internal/connector/vcarddir`: revision-aware prepare/sync/refresh orchestration.
- `internal/app/directories.go`, `internal/domain/directory.go`: connection boundary and data types.
- `internal/storage/sqlite/directories.go`: schema migration 4, ownership mappings, atomic refresh/disconnect. `contacts.go` enforces read-only ownership for ordinary writes.
- `internal/tui/{transfer,import_browser,directories}.go`: source chooser, file browser, modal workflows, startup and manual folder refresh.
- `internal/mailbridge`: TideMail protocol, automatic folder refresh, warning/read-only projections and multi-email lookup.
- `internal/cli`: versioned JSON scripting and read-only recipient output. Directory management currently has no dedicated CLI resource.

## TideMail integration

The sibling repo is `../tidemail`. Its working checkout must contain the TideContacts integration when the user starts it with `go run .`; installing a compatible executable elsewhere is insufficient. The earlier red-plugin error was a missing `contacts_write` permission in that checkout’s host code. Integration changes were restored without discarding notification work.

People marks folder contacts read-only, supports `r` refresh and shows last-good-cache warnings. Advanced CRM is the bundled matching executable. TideMail native contacts remain a separate book. Do not alter or commit the sibling’s unrelated untracked HANDOFF.md.

## Build and verification

- `make check`: formatting, build, vet and short race suite.
- `make smoke`: terminal lifecycle, contact and CRM flows.
- `make smoke-directories`: real modal folder connection, then plugin-only update, UID rename, failed-scan recovery and deletion with the TUI stopped.
- `make smoke-cli` and `make smoke-mail-clients`: executable protocol checks.
- The sibling TideMail repo has focused Contacts/TideContacts/PluginSettings and SimpleContacts tests.

Golden frames were updated for the Import/Refresh menu entries. Current builds are installed in `~/.local/bin` and the matching TideContacts plugin bundle in `~/.config/tidemail/plugins/tidecontacts`. Tests use temporary profiles. On November 20, the user reported successfully syncing a live Google Contacts account.

## Pending / limits

TideContacts PR #1 merged as `811391a`, and TideMail PR #141 merged as `e1c53ae`. The contact display and selection updates are in local commit `30b8fc5`. The Google Contacts changes are still in the working tree. Next, run `make check`, `make smoke`, and `make smoke-directories` against those changes and commit them separately.

Direct server sync for other providers, calling khard as a query command, the remaining provider-specific importers, iCalendar import, durable CRM undo, ASCII-only glyph mode and AI remain deferred. [Migration and backup](docs/migration-backup.md) covers schema 4 and shared-database precautions. The GitHub wiki repository was unavailable when checked; no wiki update was published.
