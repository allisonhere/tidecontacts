# TideContacts status

Phase 4: a standalone, offline personal CRM in the Phase 1 TideUI shell, on top of the Phase 2 address book.

Implemented in Phase 3: unified activity timeline with multi-person links and provenance; tasks with six states, seven views, quick capture, recurrence and reminders; follow-up cadences computed from recorded interactions; projects with milestones; configurable keyboard-driven pipelines with stage history; relationship explorer; People Inbox with sticky dismissal; deterministic Relationship Radar with settings; smart collections on the shared search grammar; contact dossier with link following and back-navigation; contact merge; `--due` / `--notify` for OS schedulers. All eight workspaces are functional.

Verified by unit, integration and race tests (domain, storage, TUI workflows), golden frames at four sizes per workspace, bounded-render checks down to 1×1, a measured 10,000-contact scale test, and three real-PTY smoke scripts. See docs/phase3-decisions.md for decisions, measurements and limitations.

Carried over from Phase 2: normalized UUID people/organizations, methods, memberships, relationships, tags and custom fields, revisions, history, tombstones, undo, FTS5/fuzzy search, CSV/vCard import and export.

Also: JSON export/import and iCalendar export of CRM records, and restore of deleted tasks, activities, projects and pipeline items.

Phase 4 adds an opt-in, read-only TideMail cache connector via the **Sync TideMail** palette command or `--sync-tidemail`, with Message-ID deduplication, existing-contact participant linking and `--tidemail-status`. No message bodies or attachments are copied.

Deferred: network sync, plugins, AI, iCalendar import, ASCII-only glyph mode. Mouse support focuses panes only; theme choices persist; density changes are session-only; logs do not rotate.

Demo: `--demo` opens isolated fictional data; `--demo-reset` resets that profile. Live TideMail sync and desktop notifications are disabled in demo mode.

Discoverability: wide terminals use Menu → record list → details. Ctrl+G focuses the workspace/action menu; smaller terminals show it as a modal. Empty contact lists show creation/import guidance.

Themes: searchable built-in palettes via Menu → Themes, plus Match TideMail for installed supported palettes. Matching resolves on startup.

Help now uses TideUI HelpBrowser: category/content panes, search and staged Escape navigation. Local TideUI checkout is required until this component is released.

Simple TideMail plugin: contacts.list/save/search bridge with revision checks, duplicate protection and CRM-field preservation. Paired TideMail changes provide People, compose, recent cache correspondence and Advanced CRM. See docs/tidemail-plugin.md. External address-book sync remains deferred.

Plugin settings: default contacts view, real/demo profile, read-only connection status and profile-aware Open Advanced CRM. The demo launcher cannot select live data.
