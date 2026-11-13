# TideContacts

A standalone, offline-first personal CRM for the terminal: contacts, a timeline of what happened, tasks and reminders, projects, optional pipelines, and a review inbox that explains itself. Written in Go with TideUI, Bubble Tea, Ripple and SQLite. No account, server, browser, network connection, plugin or AI service is needed.

## Build and run

Requires Go 1.26.1 or newer. SQLite is pure Go; no C compiler is required.

```sh
make build
./bin/tidecontacts
# Or:
go build ./cmd/tidecontacts
./tidecontacts
go install ./cmd/tidecontacts
```

`--check` validates configuration and initializes/upgrades the database without opening the TUI. `--due` prints what is due and exits with status 3 when something is; `--notify` sends newly due reminders to the desktop notification service once (both are for your OS scheduler; see [tasks](docs/tasks.md#reminders)). `--version` prints the milestone. Noninteractive output prints guidance and exits. The Makefile disables VCS stamping for source archives and restricted workspaces.

## First contact

Start in People, press `n`, and enter a display name. Names are optional: a nickname, contact method or organization can supply the display identity. Press F2 to save.

Use F5/F6 to move between editor sections: Identity, Methods, Organizations, Tags & fields, Relationships, Notes and Biography. Tab/Shift+Tab moves fields. Ctrl+N adds a repeated entry; Ctrl+D removes the current entry. F2/Ctrl+S saves the entire record atomically; Esc cancels and asks before discarding changes.

In Methods, choose `email`, `phone`, `website`, `social`, `messaging` or `address`. Changing the kind to address exposes its postal components. Use `true`/`false` for flags. There can be one preferred method of each kind. In Organizations, an existing name/UUID selects an organization; a new name creates it in the same save transaction. Relationship targets must already exist or be included in the same import.

Notes and biography use Ripple: multiline editing, soft wrap, Shift+arrows selection, undo/redo, bracketed paste, and clipboard integration where the OS provides it. No editor subprocess is needed.

## Browse and search

`j/k` or arrows move the contact list; the profile updates immediately. Tab and `h/l` move pane focus. `[` and `]` cycle Overview, Contact methods, Organizations, Notes, Relationships, Custom fields and History. Inside the profile, j/k scrolls; PageUp/PageDown and Home/End also work. History contains real local saves, not email activity.

`1` opens People; `2` opens Organizations; `3`–`8` open Relationships, Activities, Tasks, Projects, Pipelines and Insights (see below). `:` or Ctrl+K opens fuzzy commands; `?` shows help. Commands are also available for runtime themes, pane widths and density.

`/` starts incremental search. Enter retains the query; Esc clears it. Examples:

```text
alex
@github.com
tag:developer
org:"Riverside Studio"
is:favorite has:phone
is:archived
is:deleted
```

Normal lists exclude archived and deleted records. `a` archives/unarchives, `f` toggles favorite, `D` requests deletion confirmation, and `u` undoes the last recoverable edit/flag/deletion in the session. Search deleted records and use **Restore archived/deleted contact** in the palette to recover a tombstone. A newer external edit prevents undo or stale saves from overwriting it.

`s` cycles name, organization, recently updated and recently added sorts. Space marks contacts for export. Compact/comfortable density is configurable. List rendering is virtualized; searches run asynchronously and discard stale results.

## Personal CRM

Everything here is local and works offline. Start with the [personal CRM guide](docs/crm-guide.md).

- **Capture in one line, from anywhere.** `T` adds a task and `A` logs an activity, linked to whatever is selected: `Send proposal with:"Alex Rivera" due:fri pri:high project:Harbor`. `e` opens a full form when you want one.
- **Activities (4).** One timeline of calls, meetings, notes and custom entries, grouped by day, filterable by type, person, project, tag and date. Imported and external records keep their provenance and are read-only. [Details](docs/activities.md)
- **Tasks (5).** Inbox, Today, Upcoming, Waiting, All, By person, By project, plus your own pinned views. Complete, reopen, reschedule, snooze, prioritize, link, duplicate. Recurrence (`every 2 weeks`, `monthly on 31`, `after completion`) creates exactly one successor and is correct across daylight-saving changes. Reminders show while the app runs; `--notify` covers the rest if you schedule it. [Details](docs/tasks.md)
- **Projects (6).** People, tasks, milestones, timeline and notes in one place. **Pipelines (7).** Stages you define, moved entirely from the keyboard; a board on wide terminals, a stage list on narrow ones. [Details](docs/projects-pipelines.md)
- **Relationships (3).** A text tree of who connects to whom through relationships, organizations and projects, with follow-up cadences, important dates and relationship notes.
- **Insights (8).** The [People Inbox](docs/inbox.md) lists what needs attention and why, and dismissals stick until something changes. [Relationship Radar](docs/radar.md) shows deterministic, explainable signals — no scores, no AI. [Smart collections](docs/collections.md) are saved live filters.
- **Contact dossier.** `[` from a profile's Overview shows follow-up, open tasks, commitments, projects, connections and dates; Enter follows any of them and Esc comes back to the same place.

## Import and export

`I` starts a real import workflow: enter a CSV/vCard path, map CSV columns, inspect the dry-run preview, review duplicates, select records, and explicitly confirm. No writes occur during preview. Invalid records show errors. Duplicate names, emails and normalized phones produce warnings and are excluded by default; Space explicitly includes them as new records. Import never overwrites an existing contact. The selected batch commits atomically, with progress and a results summary.

CSV header detection can be toggled. Left/Right changes a column's mapping; repeated email/phone columns are supported. Common export headers have defaults. `b` returns from preview to mapping. Input files are limited to 50 MiB per batch.

`E` exports the filtered collection. Palette commands export **all contacts** (people and organizations, including archived, excluding tombstones) or **marked/selected contacts**. Enter a `.csv` or `.vcf` destination and confirm. Export includes private notes. Files publish atomically and **never replace an existing destination**.

vCard 3.0/4.0 import supports structured names, multiple methods, postal addresses, organizations/titles, notes, birthdays, categories and retained extension properties. Export writes vCard 4.0. CSV and vCard exports include a TideContacts extension carrying the complete aggregate for richer round-trips. See [import/export compatibility](docs/import-export.md).

## Configuration and data

Linux defaults:

- `~/.config/tidecontacts/config.toml`
- `~/.local/share/tidecontacts/tidecontacts.db`
- `~/.local/state/tidecontacts/tidecontacts.log`

Honor absolute `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_STATE_HOME`. macOS uses Library/Application Support and Library/Logs; Windows uses the OS configuration directory. New data/log files have private permissions. Missing configuration uses defaults; invalid configuration reports an error without rewriting the file.

```toml
theme = "dark" # dark, light, contrast
default_workspace = "people"
density = "compact" # compact, comfortable
log_level = "info"
mouse = false # optional click-to-focus panes

[layout]
narrow = 90
wide = 140
left = 0.23
right = 0.25

[keybindings]
quit = ["x"] # replaces q outside inputs
```

Wide terminals show three panes; medium terminals show list/profile; narrow terminals show one focused pane with Tab navigation and Esc back to the list. Shift+Left/Right adjusts the left pane; Shift+Down/Up adjusts context width. Ratios save on orderly exit and override TOML defaults. Runtime theme/density changes last for the session; TOML controls startup. `NO_COLOR` retains text focus markers. `TERM=dumb` uses plain guidance.

SQLite migrations are append-only and preserve all Phase 1 and Phase 2 data; Phase 3 only adds tables (see [migration and backup](docs/migration-backup.md)). Foreign keys, WAL, busy timeout, transactions, optimistic revisions and tombstones protect local data. Back up the database using SQLite's backup API, or copy it after the app has closed; do not copy only the main file while WAL writes are active. JSON logs currently append without rotation.

## Development

```sh
make check # formatting, build, vet, race tests
make smoke # real PTY lifecycle, contact and CRM workflow tests (Python 3, Unix)
go test ./... # includes a 10,000-contact scale test; add -short to skip it
UPDATE_GOLDEN=1 go test ./internal/tui # intentionally regenerate reviewed frames
```

See [architecture](docs/architecture.md), [keys](docs/keybindings.md), [search grammar](docs/search.md), the [CRM guide](docs/crm-guide.md) and [Phase 3 decisions](docs/phase3-decisions.md).

Deferred: network sync, a TideMail connector (the idempotent import interface exists; no connector ships), executable plugins, AI, and file export of CRM records. See [Phase 3 decisions](docs/phase3-decisions.md) for known limitations.
