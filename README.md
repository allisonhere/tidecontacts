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

### Nix

The repository is a flake. Run TideContacts without installing it:

```sh
nix run github:allisonhere/tidecontacts
```

To install it, add the flake as an input and use
`inputs.tidecontacts.packages.${system}.default` (or apply
`inputs.tidecontacts.overlays.default` and use `pkgs.tidecontacts`) in
`environment.systemPackages` or `home.packages`. You can also run
`nix profile install github:allisonhere/tidecontacts`. On Linux the package adds
`xdg-open` and `notify-send` to the end of TideContacts' `PATH`. It also ships the
TideMail contacts plugin in `share/tidemail/plugins/tidecontacts`; see
[TideMail contacts plugin](docs/tidemail-plugin.md#nix) to enable it.

`nix develop` opens a shell with Go, gopls, golangci-lint, delve and Python for
the smoke tests.

## Try fictional demo data

```sh
./bin/tidecontacts --demo
# Start over with fresh examples:
./bin/tidecontacts --demo-reset
```

Demo mode uses its own database, configuration and logs under the `demo` directory
beside your normal database. It preserves your demo edits between launches and
shows **DEMO** in the status bar. Includes fictional contacts, organizations,
relationships, activities, tasks, a project, a pipeline and reminders. Dates are
relative to the first launch. Live TideMail activity sync, vCard-folder connections and desktop notifications are
disabled. Reset affects only this demo profile.

## Scripting

TideContacts now has JSON commands for contacts, organizations, tasks, activities,
projects, pipelines and items. No running app or server is needed:

```sh
./bin/tidecontacts --demo contacts list --full
./bin/tidecontacts --demo tasks list --view today
printf '%s\n' '{"Title":"Follow up"}' | ./bin/tidecontacts --demo tasks create --input -
./bin/tidecontacts schema tasks
```

Updates require revisions (or pipeline configuration tokens); JSON errors and
exit codes are documented in the [CLI guide](docs/cli.md). `make smoke-cli` tests
the real binary using fictional data.

## First contact

Start in People, press `n`, and enter a display name. Tab moves to Email and then Phone. Editing either updates the preferred entry, or the first entry when none is preferred. Names are optional: a nickname, contact method or organization can supply the display identity. Press Ctrl+S (or F2) to save. The editor opens as a modal over the screen you were on.

Press Up from Name to focus the section tabs. Left/Right changes section and Down or Enter returns to the fields. F5/F6 also moves directly between sections: Name & contact, Emails & phones, Organizations, Tags & fields, Relationships, Notes and Biography. Down/Up or Tab/Shift+Tab moves between inputs; Left/Right moves within text. F5 is previous section and F6 is next section. Ctrl+N adds a repeated entry; Ctrl+D removes the current entry. Enter advances a field. F2 or Ctrl+S saves the entire record atomically (in Notes and Biography Enter is a newline, so use Ctrl+S); Esc cancels and asks before discarding changes.

In Emails & phones, choose `email`, `phone`, `website`, `social`, `messaging` or `address`. Changing the kind to address exposes its postal components. Use `true`/`false` for flags. There can be one preferred method of each kind. In Organizations, an existing name/UUID selects an organization; a new name creates it in the same save transaction. Relationship targets must already exist or be included in the same import.

Notes and biography use Ripple: multiline editing, soft wrap, Shift+arrows selection, undo/redo, bracketed paste, and clipboard integration where the OS provides it. No editor subprocess is needed.

## Help

Press **?** or choose **Menu → Help** for the TideUI help browser. Selected
categories use TideMail settings styling: uppercase accent text and a left focus rail. Choose a
category on the left with ↑/↓ or j/k. Right, Tab or Enter focuses the shortcut
content; left or Shift+Tab returns to categories. Scroll the content with arrows,
PageUp/PageDown, Home and End. `/` searches categories, keys and descriptions.
Enter keeps the filter; Esc clears it before closing. `?` or q closes help.

The shared help browser is included in the pinned TideUI dependency; no sibling
TideUI checkout is required.

## Themes

Open **Menu → Themes** (Ctrl+G), or search **Choose theme** in the command palette.
Move through the palette list or search to preview themes across the app.
Enter keeps the theme; Esc restores the previous choice. Unconfirmed previews
are never saved. Confirmed choices are saved on normal exit.
When a supported TideMail theme is detected, **Match TideMail** appears at the top.
It reads TideMail’s configured palette on startup; restart to pick up changes there.
Unsupported TideMail themes (including its dynamic desktop theme) are not offered.

## Finding your way

Terminals at least 120 columns wide show **Menu → record list → details**. The menu names every
workspace and lists actions for the current one. Press **Ctrl+G** to focus it,
use ↑/↓ or j/k to preview workspaces as you move, then Tab, Right, or l to enter the record list. Enter opens the highlighted workspace or runs the highlighted action. Press `n` on a workspace to open its New dialog directly (or your configured New shortcut). Esc leaves the menu. The workspace numbers, help (?), command palette (:), reminders (b), and import (I) work directly from the menu.
Left moves from details to the list and from the list to the menu. Right enters the highlighted workspace from the menu, then moves from the list into details. Tab and Shift+Tab cycle through record panes. On smaller terminals the menu
opens as a modal. Workspace number shortcuts and the command palette still work.

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

Normal lists exclude archived and deleted records. For local contacts, `a` archives/unarchives, `f` toggles favorite, `D` requests deletion confirmation, and `u` undoes the last recoverable edit/flag/deletion in the session. Search deleted records and use **Restore archived/deleted contact** in the palette to recover a tombstone. A newer external edit prevents undo or stale saves from overwriting it. Connected vCard-folder contacts are read-only; edit their original cards and refresh.

`s` cycles name, organization, recently updated and recently added sorts. Space marks contacts for export. Compact/comfortable density is configurable. List rendering is virtualized; searches run asynchronously and discard stale results.

## TideMail connector (Phase 4)

In the app, press `:` or Ctrl+K, choose **Sync TideMail**. The app finds TideMail’s
local cache automatically (`$XDG_DATA_HOME/tidemail/mail.db`, or
`~/.local/share/tidemail/mail.db`). If it is missing, a prompt lets you provide
a custom location. **Show TideMail sync status** displays the last
successful sync. Esc cancels the path prompt. Sync runs in the background.

You can also import from the command line:

```sh
./bin/tidecontacts --sync-tidemail /absolute/path/to/tidemail.db
./bin/tidecontacts --tidemail-status
```

The connector is opt-in and opens the source read-only. It copies subjects and dates,
links From/To/Cc addresses to uniquely matching existing contacts, and never creates
contacts or copies bodies and attachments. Message-IDs identify activities across
folders and repeat runs. Records without Message-IDs or valid dates are skipped.
Use Activities search `app:tidemail` to find the results. External activities are
read-only in TideContacts. Successful runs record a visible last-sync timestamp;
failed runs preserve the previous timestamp. A partial import can be retried safely.
Sync scans the cache each time; it never deletes activities missing from the cache.
There is no automatic polling, network access or TideMail write-back.

## Personal CRM

Everything here is local and works offline. Start with the [personal CRM guide](docs/crm-guide.md).

- **Create a task without command syntax.** `T` opens the New task modal anywhere; `n` does the same in Tasks. Tab through labeled fields, cycle status/priority and search existing people/projects with Space or Ctrl+O. The selected contact/project is linked automatically. One-line task capture remains an optional palette command; `A` opens the matching New activity form.
- **Activities (4).** One timeline of calls, meetings, notes and custom entries, grouped by day, filterable by type, person, project, tag and date. Imported and external records keep their provenance and are read-only. [Details](docs/activities.md)
- **Tasks (5).** Inbox, Today, Upcoming, Waiting, All, By person, By project, plus your own pinned views. Complete, reopen, reschedule, snooze, prioritize, link, duplicate. Recurrence (`every 2 weeks`, `monthly on 31`, `after completion`) creates exactly one successor and is correct across daylight-saving changes. Reminders show while the app runs; `--notify` covers the rest if you schedule it. [Details](docs/tasks.md)
- **Projects (6).** People, tasks, milestones, timeline and notes in one place. **Pipelines (7).** Stages you define, moved entirely from the keyboard; a board on wide terminals, a stage list on narrow ones. [Details](docs/projects-pipelines.md)
- **Relationships (3).** A text tree of who connects to whom through relationships, organizations and projects, with follow-up cadences, important dates and relationship notes.
- **Insights (8).** The [People Inbox](docs/inbox.md) lists what needs attention and why, and dismissals stick until something changes. [Relationship Radar](docs/radar.md) shows deterministic, explainable signals — no scores, no AI. [Smart collections](docs/collections.md) are saved live filters.
- **Contact dossier.** `[` from a profile's Overview shows follow-up, open tasks, commitments, projects, connections and dates; Enter follows any of them and Esc comes back to the same place.

## Export, import and restore for CRM records

`:` → **Export CRM records (JSON)** writes activities, tasks, reminders, projects, pipelines with items and history, follow-up schedules, dates, relationship notes and collections to a versioned file; **Export tasks and reminders (iCalendar)** writes `.ics` for other calendars; **Import CRM records (JSON)** previews the file, asks for confirmation and adds what is missing without overwriting anything. Files never replace an existing destination. Deleted tasks, activities, projects and pipeline items can be brought back: filter with `is:deleted` and run Restore. See [export and import](docs/export-import.md).

## Import and export

Press **I** or choose **Menu → Import data…** to open the import modal. Active sources appear first; planned sources follow in much dimmer text with **coming soon** labels and cannot be activated.

| Active source | Behavior |
|---|---|
| CSV contacts | Browse a `.csv`, map columns, preview and confirm a one-time import |
| vCard address book | Browse a `.vcf` or `.vcard`, preview and confirm a one-time import |
| TideContacts CRM archive | Browse a `.json`, review its counts and confirm |
| Mutt / NeoMutt aliases | Read literal alias definitions, then preview and review duplicates |
| khard / vdirsyncer | Connect a local vCard folder as a read-only, refreshable address book |
| Google Contacts | Connect a Google account (email + app password) as a read-only, refreshable address book over CardDAV |

Use ↑/↓ and Enter to choose a source. Enter opens folders or selects files, ←/Backspace goes to the parent, `p` opens direct path entry (`~/` works), and `.` toggles hidden files/folders (off initially). Mapping, preview, confirmation and results stay in the modal. Esc closes it before writing. One-time contact imports show invalid rows and leave duplicates unchecked; Space explicitly includes them as new contacts. The confirmed batch commits atomically without overwriting existing contacts. [Formats and limitations](docs/import-export.md).

For **khard / vdirsyncer**, press `n` to browse and `s` to select the current folder, or `p` to enter its path. Preview and confirm the connection. For **Google Contacts**, press `n` and enter the account email and an [app password](https://myaccount.google.com/apppasswords) (requires 2-Step Verification); the password is saved to the OS keychain, never to the database or a config file. Both refresh at TideContacts startup, from **Refresh address books** in the menu, and when the TideMail plugin loads or refreshes contacts or handles Ctrl+T lookup. TideContacts need not be running for the plugin to refresh them. A failed refresh keeps the previous snapshot; a successful one updates changed cards and hides removed cards. Local contacts stay separate. vdirsyncer continues to own remote CardDAV sync for folders it manages; TideContacts performs the CardDAV sync itself for a connected Google account. [Address book setup and recovery](docs/vcard-directories.md).

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
wide = 120
left = 0.23
right = 0.25

[keybindings]
quit = ["x"] # replaces q outside inputs
```

Wide terminals show three panes; medium terminals show list/profile; narrow terminals show one focused pane with Tab navigation and Esc back to the list. Shift+Left/Right adjusts the left pane; Shift+Down/Up adjusts context width. Ratios save on orderly exit and override TOML defaults. Theme choices are saved on exit and override the startup theme; density changes last for the session. `NO_COLOR` retains text focus markers. `TERM=dumb` uses plain guidance.

SQLite migrations are append-only; the current schema is version 4 and adds directory ownership tables while preserving existing contacts and CRM data (see [migration and backup](docs/migration-backup.md)). Foreign keys, WAL, busy timeout, transactions, optimistic revisions and tombstones protect local data. Back up the database using SQLite's backup API, or copy it after the app has closed; do not copy only the main file while WAL writes are active. JSON logs currently append without rotation.

## Extensions (optional)

TideContacts can be extended by separately installed programs that use a small,
versioned, permission-checked host API. Extensions are off by default, nothing
runs until you explicitly approve its permissions, and the Community app works
fully without any. Manage them from the palette (**Manage extensions…**) or the
JSON CLI (`extensions list`, `capabilities list`). Permissions control what the
host API returns; they are **not** an operating-system sandbox, so only install
extensions you trust. Start with the [extension guide](docs/extensions/README.md),
the [API specification](docs/extensions/specification.md) and the
[security model](docs/extensions/security.md). `make extension-demo` builds the
read-only *Relationship Digest* example.

## Development

```sh
make check # formatting, build, vet, race tests
make smoke # real PTY lifecycle, contact and CRM workflow tests (Python 3, Unix)
make smoke-directories # folder connection and plugin-only refresh in an isolated profile
make smoke-cli # versioned JSON command workflows
make smoke-extensions # extension discovery, approval, host API limits and failure isolation
make extension-demo # build the Relationship Digest example into bin/extensions
make smoke-mail-clients # aerc/NeoMutt output and shell argument checks
go test ./... # includes a 10,000-contact scale test; add -short to skip it
UPDATE_GOLDEN=1 go test ./internal/tui # intentionally regenerate reviewed frames
```

The Nix flake needs no Nix knowledge day to day. After a `go.mod` or `go.sum`
change lands on `main`, the Nix workflow (`.github/workflows/nix.yml`) opens a
`build(nix): update vendorHash` PR, and once a month it opens a
`build(nix): update flake.lock` PR; merge them when they appear. Those PRs don't
trigger other workflows on their own; close and reopen one to run them. To fix
the hash locally instead, run `bash scripts/update-nix-vendor-hash.sh` (requires
Nix). The one manual edit: when `go.mod` moves to a new Go minor version (e.g.
1.27), change both `go_1_26` references in `flake.nix` to match.

See [architecture](docs/architecture.md), [keys](docs/keybindings.md), [search grammar](docs/search.md), the [CRM guide](docs/crm-guide.md) and [Phase 3 decisions](docs/phase3-decisions.md).

Deferred: direct CardDAV/network sync for providers other than Google, calling khard as an external query command, additional provider-specific importers, and AI. See [Phase 3 decisions](docs/phase3-decisions.md) for known limitations.

## TideMail contacts plugin

A simple People view inside the updated TideMail provides search, local contact add/edit, compose,
folder refresh with **r**, read-only folder labels and recent correspondence, with Advanced CRM kept separate. See
[plugin setup and keys](docs/tidemail-plugin.md). Build with `make plugin`; install
with `make install-tidemail-plugin`. The TideMail checkout used by `go run .` must include the integration too. If the plugin is red with `unknown keys: permissions.contacts_write`, update that TideMail checkout or binary and restart; installing the plugin alone cannot add host support.

To try the paired integration with fictional data, run `make tidemail-demo-build`,
then `python3 scripts/tidemail_demo.py` and press **C**. This uses an isolated profile,
including when you open Advanced CRM.

## aerc and NeoMutt

`contacts query --format aerc|neomutt` supplies recipient candidates from the
shared address book without a running app. NeoMutt supports searched completion;
the safe aerc configuration provides a candidate list without shell-interpolated
query text. Both clients can save the current sender with the supplied `,cs` binding. See [setup, limits and examples](docs/mail-clients.md).

Recipient commands read the cached address book without refreshing external folders. Refresh in TideContacts or TideMail before querying changed folder contacts. [Mutt alias import](docs/import-export.md#mutt--neomutt-aliases) is a separate workflow.
