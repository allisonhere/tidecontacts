# Architecture

The Phase 1 shell remains the host. `cmd/tidecontacts` owns initialization, configuration, logging, terminal lifecycle and database closure. Initialization runs outside the UI loop; it delivers configuration and the `app.Contacts` repository boundary to the existing model. Bubble Tea handles raw mode, alternate screen, INT/TERM, suspend and panic recovery; the host adds SIGHUP handling.

## Domain and application

`internal/domain` contains UUID contact aggregates, method/address/membership/relationship/custom-field models, validation and normalization. Person and organization identity does not depend on email. Dates accept YYYY-MM-DD and yearless --MM-DD where appropriate. Global custom field definitions validate text/number/date/boolean/URL/enum values; unknown extension payloads are retained as JSON.

`internal/app` defines the command registry, workspace state and the consumer-owned Contacts interface. Both keys and palette entries dispatch semantic command IDs. Presentation and import/export consume this interface; SQL stays in storage. No TideMail, network client, executable plugin or AI dependency enters the core. Future adapters translate external records into these aggregates and call application operations with an explicit provenance and conflict policy.

## Persistence

`internal/storage/sqlite` appends migration 2 to the unchanged Phase 1 migration. Tables normalize base entities, person/organization details, aliases, methods and postal components, memberships, relationship types/edges, tags, custom field definitions/values, history and the FTS5 projection. UUID entity identity is stable. Foreign keys define cascade behavior for owned children and restrict removal of referenced targets. Public deletion is a tombstone, never a hard DELETE of the entity.

All saves validate a deep-copied aggregate, reserve new batch identities, resolve named memberships/targets, replace only owned child collections, record a revisioned snapshot, update search projections and commit in one transaction. Reserving identities supports cyclic relationships and forward references in imports. New organization names create organizations within that transaction. Incoming memberships/relationships survive edits to the target. Read projections expose incoming relationships and organization members without treating them as owned editable rows.

Optimistic revision checks reject stale saves. Undo restores a prior aggregate only when the current revision immediately follows it; subsequent external edits are not overwritten. Historical snapshots and tombstones remain available for later sync design. Tags and custom definitions are reusable; global field definitions cannot silently change type or choices.

The existing one-connection pool, per-connection foreign-key/busy-timeout pragmas and WAL remain. Statements bind user values. Dynamic SQL is limited to application-owned filter fragments; sort/ranking is typed application logic. An aggregate read uses a read transaction for consistency. Export reads each aggregate consistently, but does not claim a single database-wide snapshot if another process is editing concurrently.

## Search and UI state

`internal/search` is a quoted/escaped query lexer with a small validated filter language. SQLite FTS5 provides token/prefix candidates; normalized exact matching and fuzzy summary ranking supplement it. Search projections are updated in the same transaction, including dependents when an organization name changes. Summary/document reads are linear; only visible rows are rendered.

`internal/tui` extends the existing model with contact browser state, selected identity, profile view, scroll positions, marked IDs, editor state and transfer workflow. Queries debounce for 100 ms. List and detail work carries generation identities and cancellable contexts; stale responses are ignored after selection/query/workspace changes. Save/import operations serialize through a busy state. Ctrl+C during a write requests transaction cancellation and waits for the completion message. Signals cancel the shared application context and SQL transactions roll back if uncommitted.

The dedicated editor owns a deep draft, so cancellation cannot mutate a saved contact. Scalar fields use Bubbles; repeated sections add/remove rows. Ripple handles notes and biography. Untouched multiline imported scalar values are preserved when editing unrelated fields. UI handlers return commands for all SQL and file work; View is pure rendering.

TideUI provides pane/layout/row/soft-panel/ratio/theme primitives. Existing wide/medium/narrow modes, themes, palette, config and pane persistence remain. People and Organizations replace their placeholders; the other workspaces retain the shell.

## Transfer boundary

`internal/transfer` owns file parsing, mappings, dry-run validation, duplicate warnings, UUID remapping and serialization. Imports are insert-only and selected batches are transactional. A progress-capable repository method reports records written before commit, with a separate final result. Exports publish complete, synced temporary files with a no-clobber hard link. CSV/vCard extensions carry full aggregates; normal fields provide interoperability.

## Migrations and development

Append SQL migrations; never edit a released migration. Add upgrade, fresh-database, rollback and referential-integrity tests. Migration metadata and all pending migrations commit atomically. Newer schemas are refused. Tests include Phase 1 preferences surviving upgrade, aggregate round-trips, cyclic relationships, stale edits, tombstones, search, transfers, editor state, snapshots, races and real PTY workflows.

## Phase 3: personal CRM

The Phase 1 shell and Phase 2 address book are unchanged hosts. Phase 3 adds records, not a second application.

**Domain** (`internal/domain`): `crm.go` holds Activity, Task, Reminder, FollowUp, ImportantDate, Project, Milestone, Pipeline, Stage, PipelineItem, InboxItem, Signal, Collection, and the Connections/Dossier projections. `recurrence.go` is the documented rule model; `dates.go` holds calendar-date helpers. These are pure and take "today" as an argument, so DST and zone behaviour is tested without a database.

**Application boundary** (`internal/app/crm.go`): `CRM` extends `Contacts`. The TUI depends on this interface only; the SQLite store satisfies it (checked at compile time). Filters (`ActivityFilter`, `TaskFilter`) live here, not in storage.

**Persistence** (`internal/storage/sqlite`): migration 3 (`crm_schema.go`) is append-only. Records link through explicit junction tables named `<owner>_<targets>` with foreign keys and reverse indexes; an activity, task, project or pipeline item can link any number of people and organizations. Tags reuse the Phase 2 `tags` table. One FTS5 table, `crm_search`, indexes titles, text and linked names per record and is maintained in the writing transaction. Every write is a single transaction with a revision check. Deletes are tombstones. The store's clock is injectable; its location defines the local calendar.

Derived views are computed, not stored: follow-up due dates, the People Inbox, Radar signals and collection results are evaluated from current rows on each load. The only stored derivative is `inbox_state` (first-seen time, fingerprint, dismissed/snoozed/reviewed), which is what makes dismissal stick. Recurrence stores one rule per series and creates one successor on completion, guarded by a unique `(series_id, occurrence)` index.

**Search** (`internal/search`): one lexer; `ParseScope` validates filters per record type and `ParseCapture` reads quick-capture lines with the same tokens.

**TUI** (`internal/tui`):

| File | Role |
|---|---|
| `crm.go` | CRM state, async list/detail loading with sequence numbers, back-navigation stack, message handling |
| `crm_actions.go` | Contextual commands dispatched from the shared registry |
| `crm_records.go` | Record forms and quick-capture prompts |
| `crm_forms.go` | Prompt, picker, confirmation and full-screen form components |
| `crm_rows.go` | Pure builders from domain values to list and detail rows |
| `crm_view.go` | Pane rendering, virtualized windows, the pipeline board |
| `multiline.go` | The single adapter around Ripple, used by the contact editor and every CRM form |

All storage work runs in Bubble Tea commands; `Update` never touches the database and `View` only styles prebuilt rows. Lists render only the visible window. The activity timeline pages 200 rows at a time and a profile loads twelve. A followed link pushes the current workspace, view, selection and focus; Esc restores them.

Reminders are checked once a minute while the app runs. There is no daemon: `--due` and `--notify` are short-lived modes for the operating system's scheduler.
