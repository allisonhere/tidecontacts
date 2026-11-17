# TideContacts handoff

Current state: November 16, 2026. This replaces the old Phase 3 handoff. Use [STATUS.md](STATUS.md) for the current feature inventory and [README.md](README.md) for setup.

## Delivered

TideContacts has eight CRM workspaces, the TideMail activity connector, a shared TideMail contacts plugin, a JSON scripting CLI, read-only aerc/NeoMutt recipient output, and the import modal. Active import sources are CSV, vCard, CRM JSON, Mutt/NeoMutt aliases and khard/vdirsyncer folders. Enabled sources automatically sort above the nine dimmed planned providers.

Connected folders keep stable contact identities by UID, refresh at startup or explicitly, and refresh through TideMail’s contacts.list/search path while TideContacts is stopped. Complete scans apply atomically; failed scans preserve the cache. Directory contacts are read-only at the storage boundary. Disconnect hides the records and retains mappings for reconnect. No source files are written and no direct CardDAV or external khard command is implemented. [Full behavior](docs/vcard-directories.md).

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

Golden frames were updated for the Import/Refresh menu entries. Current builds are installed in `~/.local/bin` and the matching TideContacts plugin bundle in `~/.config/tidemail/plugins/tidecontacts`. Tests use temporary profiles. No live address-book connection was added during development.

## Pending / limits

Importer/directory/form work is on a dedicated TideContacts feature branch. The TideMail integration is being prepared separately in `/tmp/tidemail-tidecontacts-pr` from current origin/main, using only the contact integration commit. Do not claim a release has shipped. Inspect both working trees before committing and preserve unrelated work. The current request authorizes committing/pushing these feature branches and opening PRs, while preserving the original TideMail checkout and its independent work. It does not authorize merging or tagging releases.

Direct server sync, calling khard as a query command, the remaining provider-specific importers, iCalendar import, durable CRM undo, ASCII-only glyph mode and AI remain deferred. [Migration and backup](docs/migration-backup.md) covers schema 4 and shared-database precautions. The GitHub wiki repository was unavailable when checked; no wiki update was published.
