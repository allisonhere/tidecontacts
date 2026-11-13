# TideContacts status

Phase 3: a standalone, offline personal CRM in the Phase 1 TideUI shell, on top of the Phase 2 address book.

Implemented in Phase 3: unified activity timeline with multi-person links and provenance; tasks with six states, seven views, quick capture, recurrence and reminders; follow-up cadences computed from recorded interactions; projects with milestones; configurable keyboard-driven pipelines with stage history; relationship explorer; People Inbox with sticky dismissal; deterministic Relationship Radar with settings; smart collections on the shared search grammar; contact dossier with link following and back-navigation; contact merge; `--due` / `--notify` for OS schedulers. All eight workspaces are functional.

Verified by unit, integration and race tests (domain, storage, TUI workflows), golden frames at four sizes per workspace, bounded-render checks down to 1×1, a measured 10,000-contact scale test, and three real-PTY smoke scripts. See docs/phase3-decisions.md for decisions, measurements and limitations.

Carried over from Phase 2: normalized UUID people/organizations, methods, memberships, relationships, tags and custom fields, revisions, history, tombstones, undo, FTS5/fuzzy search, CSV/vCard import and export.

Deferred: sync, a TideMail connector, plugins, AI, export of CRM records, in-app undelete for CRM records, ASCII-only glyph mode. Mouse support focuses panes only; theme/density changes are session-only; logs do not rotate.
