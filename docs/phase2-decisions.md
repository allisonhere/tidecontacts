# Phase 2 decisions

## Reuse and inspection

Read the repository's Phase 1 implementation and modern-tui guidance; no repository AGENTS.md was present. Reused its command registry, configuration, logging, migrations, themes, responsive layout and terminal lifecycle. Inspected the pinned TideUI public APIs and existing TideMail contacts/vCard code.

TideMail currently stores a flat, email-keyed contact with one phone, organization, title and note. TideContacts uses UUID entities and multiple methods; a future adapter can map TideMail addresses to methods and preserve source metadata. Email is never the TideContacts key. No direct TideMail dependency was introduced.

TideUI has the needed shell/row/modal/ratio primitives but no full contact editor. Bubbles is already its single-line input dependency. Ripple `v0.5.1-0.20261105022651-9b597652c893`, the cached revision used by TideMail, provides real multiline editing, selection, wrapping, undo/redo and paste. It is used for notes/biography. Ripple Find is disabled because host Tab/Esc navigation owns those keys; no nested search modal is needed here.

The MIT emersion/go-vcard revision `v0.0.0-20241024213814-c9703dde27ff` matches TideMail's tested parser. Its implementation/license and v3/v4 behavior were reviewed locally. We do not claim a network-verified latest release. Standard-library encoding/csv handles CSV. No browser, web framework or network service was introduced.

## Choices

- Normalized tables with a shared entity identity enable people, organizations, all three relationship combinations and separate memberships without polymorphic foreign-key gaps.
- Complete aggregate transactions keep method preferences, tags, fields, history and search projections consistent. Revision checks make concurrent editing explicit.
- Soft deletion preserves history and references for future sync. Undo covers the latest recoverable edit/flag/deletion in the current session; durable restore is available through tombstone search and the palette.
- A full-screen sectioned editor avoids chains of small dialogs. Exact organization/relationship names or UUIDs resolve links. New organization names create records; ambiguous names fail with an actionable error.
- Import duplicates are unchecked by default and never merged/overwritten. The user can explicitly create separate duplicates. Whole selected batches roll back on any storage failure.
- Export refuses existing paths, including racing destinations. Full aggregate extensions preserve data beyond portable vCard/CSV fields.
- Config adds compact/comfortable density. Runtime density/theme remain session preferences, consistent with Phase 1.

## Verification and bounds

Tests cover normalized aggregate storage, upgrade, rollback, foreign keys, cycles, revisions, tombstones, parser errors, FTS/fuzzy/normalized matches, Unicode, transfer round-trips, duplicate warnings, no-clobber export, draft cancellation, notes, undo, stale results and viewport bounds. Frames cover 160×45, 110×32, 80×24 and 60×20. Real PTY tests exercise create, methods, multiline notes, memberships, favorite/archive/undo, export, duplicate-reviewed import and reopening.

A 3,000-contact fuzzy-search benchmark exposed an FTS join scan. Replacing it with one document pass reduced measured search time from roughly 1.37 seconds to 11 ms on the development machine. This is a local benchmark, not a universal latency guarantee. UI rendering remains virtualized; summary results are held in memory.

## Limitations

- This phase has no sync, mail activity, graph visualization, deduplication/merge engine, plugin execution or AI. The six other workspace shells are deferred.
- The editor supports link entry by exact name/UUID, not an autocomplete directory. Tag and field values are edited in-record; there is no separate global taxonomy administration screen. A custom field's global type/options are immutable once defined.
- History is an audit of local aggregate saves; it is not a general time-travel editor. Creation/import does not provide a one-key batch undo; individual records can be tombstoned.
- Import is bounded to 50 MiB per file. Preview reads records in memory. Vendor-specific legacy vCards may need conversion; see import-export.md.
- Avatar references are stored but not fetched/rendered. Mouse remains click-to-focus only. Logs do not rotate automatically.
- Forced process termination may discard a draft. Completed saves are transactional. Normal Esc/Ctrl+C warns on unsaved editor changes.
- Existing sandbox constraints still require disabled VCS stamping and temporary Go/lint caches. Network vulnerability auditing is unavailable in this environment; no clean vulnerability audit is claimed.
