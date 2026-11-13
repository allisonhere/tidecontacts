# Phase 3 decisions

## Inspection

- No `AGENTS.md` exists in this repository or its parent. The `modern-tui` skill was loaded; only its `SKILL.md` is installed (the `references/` files it mentions are not present), so its top-level conventions were followed.
- Phases 1 and 2 were verified rather than assumed: the tree built and all tests passed before any change. The repository is not under version control, so a tarball of the Phase 2 tree was taken before work began.
- TideUI was inspected through the pinned module (`v0.3.2-0.20261108000219`). TideMail's `internal/db/contacts.go`, `internal/plugin/contacts.go` and `internal/pluginquery/analytics.go` were read: contacts there are keyed by email address and analytics by Message-ID, which shaped the external-activity interface below.

Prerequisites found missing and added because Phase 3 could not be safe without them:

1. **Contact merge.** Phase 2 deferred it; Phase 3 must not leave references broken by a merge, so `MergeContacts` and a palette command now exist.
2. **Key-binding compatibility.** New default keys would have invalidated existing `[keybindings]` files (the README's own `quit = ["x"]` example collides with dismiss). Phase 3 defaults are therefore "soft": a user's binding wins.
3. **An injectable store clock**, needed to test recurrence, reminders and inbox rules.

## TideUI reuse

Reused as-is: `Renderer`, `Layout` (ThreeColumn, SidebarOnly, Tabbed), `Pane` with `ScrollOffset`, `StatusBar`, `Row`, `SoftRow`, `SoftPanel`/`SoftPanelOverlay`, `RenderSoftBody`, `RenderSoftHints`, `RenderSectionDivider`, `VisibleRange`, `PaneRatio`, themes and contrast helpers. Nothing in TideUI was modified or copied, and no second visual framework was introduced.

Built in the application, from those primitives: the quick-capture prompt, the searchable picker, the record form and the pipeline board. TideUI's `Workspace`/panel system, `CommandPalette`, `Task`, `AgendaItem` and `Note` widgets were considered and not adopted: the existing shell already owns layout, palette and key routing, and the widget types are dashboard summaries rather than editable lists. A generic single-line prompt and a picker would be reasonable upstream candidates once a second Tide application needs them.

## Ripple

Ripple (already a Phase 2 dependency, same pinned revision) is used for every multi-line field: contact notes and biography, activity notes, task descriptions, project notes, pipeline item notes and relationship notes. It gives soft wrap, selection, undo/redo and paste that Bubbles' textarea does not match. It now sits behind one adapter, `internal/tui/multiline.go`; the Phase 2 contact editor was moved onto the adapter, so there is exactly one multi-line input system. Single-line fields remain Bubbles `textinput`, as in Phases 1 and 2. Ripple's Find stays disabled because the host form owns Tab and Esc.

## Schema and architecture

- **One append-only migration** (version 3) that only adds tables. Tested against a real Phase 2 database, including rollback of a failing later migration.
- **Explicit junction tables** (`activity_entities`, `task_entities`, `project_entities`, `item_entities`, and the project/task/activity cross links) with foreign keys both ways and reverse indexes. People and organizations share the Phase 2 `entities` identity, so one junction serves both.
- **Contacts stay tombstones.** Because a contact row is never hard-deleted, deleting or archiving one cannot dangle a link. Merge re-points every link in one transaction and is checked with `PRAGMA foreign_key_check`.
- **Derived, not stored.** Follow-up due dates, inbox entries, Radar signals and collection results are computed on read. This keeps them correct after any edit and makes collections live. The exception is `inbox_state`, which exists only to remember first-seen time and dismiss/snooze/review decisions against a fingerprint.
- **Recurrence** is a small documented rule (`recurrence_rules`), not RRULE. One successor is created on completion, guarded by a unique `(series_id, occurrence)` index; missed occurrences are skipped. This is bounded and idempotent by construction.
- **Dates.** Due dates are local calendar dates and due times are wall-clock times, so recurrence is date arithmetic and is DST-safe. Instants (activities, reminders) are stored in UTC with their zone.
- **One search grammar.** `search.ParseScope` gives each record type its own filter vocabulary over the Phase 2 lexer. Quick capture reuses the same tokens. Collections store that text.
- **Follow-ups never infer.** With no cadence and no explicit date, nothing is due. Interactions are activities of interaction types dated now or earlier; notes, task completions, project updates and future events do not count.
- **External activities** are upserted by `(source application, external reference)`, merge links additively and are never deleted by the store on a source's behalf. Local edits to them are refused.
- **Reminders without a daemon.** In-app polling once a minute; `--due` and `--notify` for the OS scheduler. `--notify` records what it announced so a repeating timer does not repeat a notification.
- **UI.** A single generic list/detail model serves six workspaces; rows are built off the UI loop and `View` only styles them. A navigation stack gives link following and Esc-back. CRM lists get a wider left pane than the contact list because their rows carry dates and people.
- **Type and state indicators are text** (`[x]`, `mtg`, `late`, `!!!`), so they survive NO_COLOR and narrow fonts. Line-drawing characters are used for the relationship tree and the board rule.

## Verification

Run on the development machine (Go 1.27.4, Linux):

| Check | Result |
|---|---|
| `go fmt ./...` | no changes |
| `go vet ./...` | clean |
| `go test ./...` | pass (7 packages with tests) |
| `go test -race -short ./...` | pass |
| `go build ./cmd/tidecontacts` | builds |
| `make smoke` scripts (3, real PTY) | pass |

Coverage added: recurrence parsing and next-date rules; DST and zone boundaries; date words; follow-up reasons; activity CRUD, multi-person links, filters, paging, provenance and idempotent upsert; task states, views, snooze, completion idempotency, reopen, duplicate, recurrence successor; reminder snooze, dismiss and recurring advance across a DST change; project CRUD, members, milestones; pipeline stage movement, history, rename, reorder, removal rules; connections and dossier; inbox deduplication, dismissal and reappearance; Radar signals and settings; live collections; merge, archive and delete integrity; migration from Phase 2. TUI tests drive real key sequences for capture, forms, pickers, confirmation, input ownership, dossier link following, back-navigation and focus restoration, the board on wide and narrow terminals, and bounded rendering for every workspace, view, overlay and form from 160×45 down to 1×1, with golden frames at four sizes.

### Measured performance

`TestListQueriesStayFastAtScale` seeds 10,000 contacts, 5,000 tasks, 5,000 activities and 300 follow-up schedules, then times the interactive paths. One run on the development machine:

| Operation | Time |
|---|---|
| Timeline page (200 rows) | 1.7 ms |
| One person's timeline | 2.8 ms |
| Today view | 14.5 ms |
| Task full-text search | 3.7 ms |
| Contact dossier | 0.8 ms |
| People Inbox | 46.8 ms |
| Relationship Radar | 44.1 ms |
| Render a 2,500-row task list | 2.9 ms |

These are single measurements on one machine, not guarantees. The test fails if they exceed generous limits (100 ms to 1.5 s). Hundreds of projects and multiple pipelines were not separately benchmarked; their queries are indexed and capped but unmeasured at that size.

## Known limitations

- **No connector ships.** The import interface for external activities exists and is tested, but nothing calls it from TideMail or anywhere else, and there is no file import for activities or tasks.
- **CRM records are not exported.** CSV/vCard export still covers contacts only. The database file is the only complete backup.
- **No in-app undelete** for tasks, activities, projects and pipeline items. Deletes are tombstones in the file, but only contacts have a restore command. Session undo (`u`) still covers contacts only.
- **Reminders while closed** depend entirely on a scheduler you set up. `--notify` supports Linux (`notify-send`) and macOS (`osascript`); it was checked by hand on Linux with a stand-in `notify-send` (one notification across two runs), the macOS path is untested, and only `--due` is covered by an automated smoke test. Windows has `--due` only.
- **Time zone** is the machine's local zone; there is no configuration setting to override it. A task can name its own zone for its due time. Reminder checks run once a minute while the app is open.
- **Recurrence** covers daily, weekly (with weekdays), monthly (by day of month), yearly, intervals, `until`, a count, and after-completion. It does not cover "second Tuesday of the month", multiple times per day, or exceptions.
- **Link fields in forms** take exact names or IDs, as in Phase 2; the searchable picker (`L`) is the easier path and links one record at a time. Roles can be typed in the project form only.
- **Pipelines** cannot be deleted, only archived. A stage with items cannot be removed. Items cannot move between pipelines.
- **Relationship explorer** is a two-level text tree plus flat views; there is no free-form graph view. Relationship labels are free text per edge; per-edge dates are not modelled (important dates belong to the contact).
- **Radar thresholds** are global, not per person. Follow-up cadences are per person.
- **Activity visibility** (`private`/`shared`) is stored and editable but not yet enforced by anything, since nothing exports activities.
- **Collections grouping** is limited to the keys listed in the reference; sort keys exist for people and tasks only.
- **Glyphs.** State indicators are ASCII, but section rules, the tree, `↗`, `⇣`, `↻` and the board marker are Unicode. There is no ASCII-only mode yet; `TERM=dumb` still gets plain guidance instead of the TUI.
- **Golden frames** for CRM workspaces cover the first view of each workspace at four sizes; other views are covered by bounds checks and content assertions rather than snapshots.
- Carried over: mouse support only focuses panes; theme and density changes last for the session; logs do not rotate. `golangci-lint` and a vulnerability audit were not run in this phase.

## Suggested starting point for Phase 4

1. **A TideMail connector** on top of `UpsertExternalActivity`: map Message-IDs to external references and addresses to contacts, read-only, opt-in, with a visible "last synced" state. The interface and its idempotency tests are already in place.
2. **Export and import of CRM records** (JSON for a full round trip, iCalendar for tasks and reminders), which also gives `visibility` a purpose.
3. **Undelete and durable undo** for CRM records, using the existing tombstones.
4. Then smaller items: an ASCII glyph mode, a time-zone setting, richer recurrence, per-edge relationship dates.
