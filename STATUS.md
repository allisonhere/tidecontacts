# TideContacts status

TideContacts is a standalone, offline-first terminal CRM with a shared TideMail address-book plugin. All eight workspaces are functional. The current SQLite schema is **4**.

## Working

- **Activity creation:** `A`, Activities `n`, Log meeting/call/note and follow-up recording open the labeled activity modal, with searchable links and preserved context. One-line activity capture remains optional in the palette.
- **Task creation:** `T` anywhere and `n` in Tasks open the labeled task modal, with status/priority choices and searchable person/organization/project links. Conversion preserves source links. One-line task capture remains an optional palette command.
- **Contacts and CRM:** people, organizations, methods, memberships, relationships, notes, custom fields, revisions/history, tombstones, undo, search, contact merge, activity timeline, tasks/recurrence/reminders, follow-ups, projects, pipelines, dossier, People Inbox, Radar and collections. [CRM guide](docs/crm-guide.md)
- **Import modal:** source chooser, file browser with `.` hidden-file toggle (off by default), path entry, mapping, dry-run preview, confirmation and results. Five active sources appear first: CSV, vCard, CRM JSON, Mutt/NeoMutt aliases, and khard/vdirsyncer folders. Nine planned provider entries are dimmed and disabled. [Import guide](docs/import-export.md)
- **Connected vCard folders:** confirmed connections, startup/manual/plugin refresh, stable UID identity, atomic updates/removals/reconnect, read-only storage protection and last-good recovery on scan failure. Local contacts remain separate; source files are never written. [Folder guide](docs/vcard-directories.md)
- **TideMail plugin:** simple People view, local add/edit, search, compose, recent correspondence, read-only folder labels, `r` refresh, Ctrl+T external lookup with all email addresses, and bundled Advanced CRM. The standalone app need not run. Settings cover default view, real/demo profile and connection checks. TideMail local contacts stay separate. Requires a compatible TideMail source/binary. [Plugin guide](docs/tidemail-plugin.md)
- **TideMail activity connector:** opt-in read-only cache scan, Message-ID deduplication, participant linking to existing contacts and last-sync status. No bodies/attachments or automatic contact creation. [README](README.md#tidemail-connector-phase-4)
- **Scripting:** versioned JSON commands for contacts, organizations, tasks, activities, projects, pipelines/items, CRM archives and schema discovery. Revision/token checks prevent stale writes. Directory-owned contacts are read-only. [CLI guide](docs/cli.md)
- **aerc / NeoMutt:** read-only recipient queries. NeoMutt supports searched completion; the safe aerc configuration returns a candidate list because upstream substitutes query text into a shell without quoting. These commands query cached contacts and never refresh folders. [Mail-client guide](docs/mail-clients.md)
- **Forms:** creation/edit forms keep the same vertical height with footer hints at the bottom, shrinking for short terminals; input rows are single-spaced across contacts/organizations, tasks, activities, projects and pipelines. Up/Down moves between single-line inputs; Left/Right edits text or changes choices. Contact-editor hints label F5 as previous section and F6 as next section.
- **Navigation and appearance:** three columns at 120 columns, menu/modal navigation, TideUI help browser, searchable themes with live preview and Match TideMail for supported installed palettes.
- **Demo:** isolated fictional contacts/CRM, paired TideMail demo, reset support. Live activity sync, folder connections and desktop notifications are disabled.

## Remaining limits

Direct CardDAV/server synchronization, calling khard as a query command, provider-specific importers shown as coming soon, iCalendar import, durable CRM undo, ASCII-only glyph mode and AI remain deferred. vdirsyncer continues handling remote sync. Mouse support focuses panes only; density is session-only; logs do not rotate. Sender capture is not included.

## Verification and installation

Unit, integration and race tests cover domain/storage/CLI/TUI/bridge behavior, migrations, optimistic conflicts, directory ownership, updates, deletions, disconnect/reconnect and failed scans. Golden frames and bounded-render tests cover terminal layouts. Real terminal and executable smoke scripts cover CRM, imports, scripting, recipient formats and folder refresh with TideContacts stopped.

`make check` runs formatting, build, vet and short race tests. `make smoke`, `make smoke-directories`, `make smoke-cli` and `make smoke-mail-clients` exercise isolated profiles. The TideMail integration has its own focused tests in that repository. See [architecture](docs/architecture.md) and [migration/backup](docs/migration-backup.md).

The current features are built and installed locally. Importer, directory and form updates are prepared on a dedicated feature branch for PR review; local availability does not imply a published release.
