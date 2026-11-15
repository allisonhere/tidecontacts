# Handoff

> Generated: 2026-11-14
> Project: TideContacts (`/home/allie/Projects/tidecontact`), Go terminal personal CRM
> Session summary: Implemented Phase 3 (personal CRM), then CRM export/import/restore, then converted all editing surfaces to TideMail-style modals. Everything is finished and verified; nothing is in progress.

## Goal

TideContacts is an offline-first, terminal-native personal CRM in Go (Bubble Tea + TideUI + Ripple + pure-Go SQLite). Phase 3 turned the Phase 2 address book into a CRM, and the original spec is fully implemented. The user's next direction is undecided: the candidate is Phase 4 (see Next Steps). Do not start new work until the user says what they want.

## Current State

**Working (verified at the end of the session):**
- `go fmt`, `go vet ./...`, `go test ./...` (and `-short`), `go test -race -short ./...`, `go build ./cmd/tidecontacts` all pass.
- `make smoke`: three real-PTY scripts in `scripts/` (`terminal_smoke.py`, `contacts_smoke.py`, `crm_smoke.py`) all pass.
- All eight workspaces are functional: People, Organizations, Relationships, Activities, Tasks, Projects, Pipelines, Insights (People Inbox, Relationship Radar, smart collections, reminders, follow-ups). Contact profiles have Dossier and Timeline views.
- CRM JSON export/import (insert-only, one transaction), iCalendar export of tasks and reminders, and restore of deleted tasks/activities/projects/pipeline items via the `is:deleted` filter plus the `restore` command.
- Every editing surface is a modal: the contact editor, all record forms, prompts, pickers and confirmations. Enter saves single-line fields; Ctrl+S saves inside multi-line fields.
- CLI: `--check`, `--due` (exit 3 when something is due), `--notify` (desktop notification via `notify-send`/`osascript`), `--version` (prints `phase3`).

**Broken:** nothing known.

**Not done (by design):** TideMail connector, iCalendar import, durable undo for CRM records, ASCII-only glyph mode, time-zone setting. The import/export *wizard* (contacts CSV/vCard, `internal/tui/transfer.go`) is still full-screen. The user did not ask to change it.

## Files in Play

| File | Why It Matters |
|------|---------------|
| `internal/domain/crm.go`, `dates.go`, `recurrence.go`, `archive.go` | Pure CRM types, calendar/DST helpers, the recurrence rule model, the export archive format |
| `internal/app/crm.go` | `CRM` interface the TUI depends on; filter types (`ActivityFilter`, `TaskFilter`) |
| `internal/app/commands.go` | One command registry. Phase 3 commands are in `phase3` with `Soft: true` (a user's own keybinding wins over these defaults) |
| `internal/config/config.go` | Keybinding validation, including soft-key precedence |
| `internal/search/query.go` | One lexer. `ParseScope(scope, q)` gives per-record filter vocabularies; `ParseCapture` handles quick capture |
| `internal/storage/sqlite/crm_schema.go` | Migration 3 (append-only, adds tables only). Junction tables `<owner>_<targets>` |
| `internal/storage/sqlite/{crm,activities,tasks,projects,pipelines,people,inbox,crmarchive}.go` | Repositories. `crm.go` has link/junction/FTS helpers; `crmarchive.go` has ExportCRM/ImportCRM; `activities.go` also has `Restore` |
| `internal/transfer/transfer.go`, `crm.go` | `PublishFile` (atomic, no-clobber), archive read/write, iCalendar writer |
| `internal/tui/crm.go` | CRM state, async loading with sequence numbers, nav stack for Esc-back, message handling |
| `internal/tui/crm_actions.go` | Contextual commands; export/import/restore handlers |
| `internal/tui/crm_records.go` | Record forms and quick-capture prompts |
| `internal/tui/crm_forms.go` | Prompt/picker/confirm/form state and keys; `overlayPanel` and `formPanel` (modal rendering) |
| `internal/tui/softform.go` | Shared TideMail-style modal renderer (`softUI`): rail, label column, filled inputs, ‹› choices, fitted hints |
| `internal/tui/editor.go` | Contact editor; `editorPanel` renders it as a modal |
| `internal/tui/crm_rows.go`, `crm_view.go` | Row builders and pane/board rendering |
| `internal/tui/multiline.go` | The single Ripple adapter, used for every multi-line field |
| `internal/tui/model.go` | Host model: Update, dispatch order, View, status bar |
| `cmd/tidecontacts/main.go` | Startup; `--due` / `--notify` (`report()`) |
| `internal/tui/modal_test.go` | Colour-enabled bounds test for every modal across themes and sizes |
| `docs/phase3-decisions.md` | Decisions, measurements, limitations, suggested Phase 4; read this first |
| `docs/crm-guide.md`, `export-import.md`, `keybindings.md`, `search.md`, `architecture.md` | User and reference docs, all current |

## What Has Been Tried (and Why It Failed)

### Attempt 1: first modal conversion (compact one-line rows in a plain SoftPanel)
- **What:** Wrapped the existing editor/form content in `SoftPanelOverlay` with clipped label/input rows.
- **Why it was rejected:** The user reverted it. It did not match TideMail's actual modal style, because TideMail's code was never inspected. The final version copies TideMail's patterns (`tidemail/internal/ui/soft_panel.go`, `form_render.go`, `contact_manager.go`): rail, label column, filled fields, ‹› pickers, lowercase hints, Enter saves.
- **Lesson:** For any UI convention the user attributes to TideMail, read `/home/allie/Projects/tidemail/internal/ui/` first.

### Attempt 2: the revert done by hand
- **What:** Restored the full-screen views by rewriting them from memory, because there is no git.
- **Why it was risky:** Without version control a revert is reconstruction, not a rollback. No defect resulted, but this is why `git init` keeps coming up.

### Smaller pitfalls already fixed (don't reintroduce)
- Overlay callbacks receive a later `*Model`. Chained pickers must use the `m` passed into the callback, not a captured outer model (link picker bug, fixed in `linkPicker`).
- SQL alias collisions in shared filter fragments (`tags t` vs `tasks t`). Use distinct aliases (`tg`, `pj`).
- In tests, `deliver()` skips commands slower than 350 ms (cursor-blink ticks). Search debounce ticks (100 ms) must be delivered after each keystroke.
- Copying a contact into another test DB must clear `Memberships`/`Relationships`, or foreign keys fail.

## Current Best Theory

The project is in a clean, complete state. If the user wants Phase 4, the highest-value item is a read-only, opt-in TideMail connector built on `Store.UpsertExternalActivity` (idempotent by `(source_app, external_ref)`; it never deletes). TideMail keys contacts by email (`tidemail/internal/db/contacts.go`) and threads by Message-ID (`internal/pluginquery/analytics.go`).

## Next Steps

1. Ask the user what they want next. Do not start anything on your own (see Key Constraints).
2. Re-offer, once, to `git init` and commit the tree (with the user's approval). There is no version control; the only Phase 2 backup was a session-temp tarball that is probably gone.
3. If the answer is Phase 4: read `docs/phase3-decisions.md` → "Suggested starting point for Phase 4", then TideMail's `internal/db/contacts.go` and `internal/plugin/query.go`, and propose a connector design before coding.
4. If the answer is the import/export wizard as a modal: `internal/tui/transfer.go` (`transferView`) is the only remaining full-screen surface. Render it through `softUI` in `softform.go` and hook it into `overlayPanel` in `crm_forms.go`, the same way as `editorPanel`.
5. Verify any change with: `go vet ./... && go test -short ./... && make build && make smoke`. The full `go test ./...` adds a roughly 45-second 10,000-contact scale test.

## Key Constraints

- **Do not work while the user is away.** The user's Fable time is limited. After a usage-limit reset or an idle gap, stop at a natural point and wait; don't volunteer extra steps (git init, polish). This is saved in memory as `feedback-dont-work-while-away`.
- No LinkedIn or other posting work. The user explicitly dropped it.
- Don't run against the user's real database without a warning. The first launch migrates `~/.local/share/tidecontacts/tidecontacts.db` to schema v3, which a Phase 2 build refuses. Tell them to back it up first.
- Offline only: no telemetry, no network code, no AI dependency. Nothing leaves the machine except user-initiated exports, `o` (open link) and `--notify`.
- UI conventions: TideUI primitives plus the user's `modern-tui` skill (only its `SKILL.md` is installed; the `references/` files are missing). The Tide-family modal style comes from TideMail.
- Migrations are append-only; never edit `contactsSchema` or `crmSchema`. Add a migration 4.
- The Makefile disables VCS stamping (`-buildvcs=false`); the repo has no `.git`.
- Model guidance the user accepted: routine follow-on work runs fine on Sonnet; reserve Fable/Opus for design-heavy steps.

## Memory Pointers

- Observations: #3029 (storage/TUI architecture constraints Phase 3 respects), #3032 (SQLite migration strategy and contacts schema), #3114 (Phase 3 default keys are soft), #3068 (recording an interaction clears an arrived follow-up date), #3058 (activity repository: paging and idempotent external upserts), #3100 (picker closed on first Enter; stale-model chaining bug).
- Searches worth re-running: `tidecontacts modal TideMail soft panel`, `tidecontacts export import restore archive`.
