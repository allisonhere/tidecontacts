# Phase 3B readiness: what a Professional sales extension needs from the host

> Status: **the gaps below are now addressed** by the Phase 3B host changes
> (see [specification §9](specification.md#9-rich-contributions-added-after-phase-3a-still-api-version-1-additive)
> and ADR-9 to ADR-11). The analysis is kept as the record of why. Originally: analysis, written against the Phase 3A source. It contains **no
> Professional business logic** and does not change any license. Professional
> implementation is blocked until a separate private repository exists (see
> [licensing-boundary.md](licensing-boundary.md) and
> [COMMERCIAL-LICENSING.md](../../COMMERCIAL-LICENSING.md); the
> [clearance checklist](../licensing-checklist.md) is entirely unchecked and the
> repository has no `LICENSE` file).

## Verified starting point

Phase 3A works end to end (discovery, approval, host API, events, storage,
palette, dossier panel, CLI, failure isolation; see the test list in
`HANDOFF.md`). It is **not merged**: it is six local commits on
`feat/phase3a-extension-architecture` with no pull request. A Professional
extension targets extension API v1 and should not start before that is reviewed
and merged.

## Gaps between Phase 3A and a sales CRM

Each item is a host (Community, public) change a Professional extension would
depend on. None is Professional business logic. Ordered by how much they
constrain the design.

1. **UI contributions are too thin.** The only slot is `contact.dossier`, and
   `Content` is labelled text lines. A sales workspace needs a workspace/nav
   entry that appears only when the extension is enabled, list and board views
   (stage columns, cards with amount, probability bar, age, attention marker),
   KPI tiles, detail/dossier sections, and modal forms with validated fields and
   searchable contact/organization selectors. Extensions must stay declarative:
   the host renders, owns focus, keys and theme; actions call back into
   `command.run`. This needs a new, versioned UI schema (ADR required). The
   existing navigation stack (previous workspace, selection, scroll, focus) must
   cover extension views.
2. **Storage is a 1 MiB key-value file.** 10,000 opportunities, indexes,
   transactions, history and reports need a real store. ADR-4 anticipated an
   optional `storage.sql`. Decision needed: an extension-owned SQLite file in
   `extension-data/<id>/` opened by the extension (simple, same-user trust, no
   host mediation) versus a host-mediated query API (more control, more surface).
   Either needs a host-run migration runner (versioned, transactional), backup
   and restore, and an explicit cross-store consistency statement: core and
   extension databases cannot be committed atomically, so workflows that touch
   both need an ordered write plus a reconciliation step.
3. **Merge/delete events lose information.** `contact.merged` carries only the
   surviving id, so an extension cannot re-point references from the merged-away
   contact. There is no `organization.deleted`, contact delete/restore, or
   project/task archive event. References from extension records to core records
   would silently dangle.
4. **Extensions cannot publish events.** Namespaced events (`opportunity.*`,
   `customer.*`, `product.*`) need an `events.publish` permission, namespace
   rules in the event table, and delivery rules that keep the existing loop
   guard (extension-sourced events are never delivered to extensions today,
   which would also block extension-to-extension use). Phase 3D automation
   depends on this.
5. **No extension-registered CLI resources.** `extensions run` returns
   host-rendered `Content`, not arbitrary versioned JSON. `tidecontacts
   opportunities …` needs manifest-declared CLI resources with schemas, routed to
   the extension, returning structured data, error codes and exit codes through
   the existing envelope. Business rules must live in the extension service, not
   in CLI handlers.
6. **One process per call is too slow for interactive boards and imports.**
   Loading a board, search-as-you-type and a 10,000-row import do not fit a 10 s
   spawn-per-request model (hard cap 30 s). ADR-1 deferred a long-lived mode;
   measure first, then add a supervised persistent process with the same
   protocol and timeouts per request.
7. **No Radar / People Inbox extension point.** The brief asks to reuse them;
   both are computed inside the SQLite store with no contribution hook. A
   `signals` contribution (typed, explainable, dismissible/snoozable, linked to
   records) is needed so Professional attention signals appear in the existing
   Inbox instead of creating a competing system.
8. **Core links cannot reference extension records.** `domain.Ref` kinds and the
   junction tables are core-owned. Keep the link on the extension side
   (opportunity → core UUIDs) and add dossier slots for the screens that should
   show opportunities (contact and organization exist partly; task, activity and
   project detail do not). Tasks/activities created "from an opportunity" are
   ordinary core records created through the host API.
9. **Entitlements need a development provider outside tests.** The brief calls
   for development/test entitlements. The interface exists and only a Community
   provider ships. A dev provider must be an explicit opt-in (for example an
   environment variable), documented as non-production and never as a license
   mechanism.
10. **No distribution story.** Installing a private extension, version
    compatibility with the host, and update/verify are all manual (as
    designed). A signed-release story is a licensing/legal decision, not only a
    technical one.

## Decisions that belong to the Professional repository

Kept out of the public repository by design: the data model, pipelines and
templates, financial representation and forecasting formulas, intelligence
rules, reports, import/export formats, demonstration data and the Professional
documentation. The financial representation (integer minor units per ISO 4217
with an exact decimal type for quantities, basis points for probability) is
recorded here only because the host API will carry such values; it is not
implemented.

## Proposed order once the private repository exists

1. Review and merge Phase 3A.
2. Host work in this repository, each its own reviewable commit and ADR: UI
   contribution schema, extension SQL storage with migrations and backup,
   richer events and `events.publish`, extension CLI resources, signals
   contribution, dev entitlement provider, optional long-lived process mode.
3. Professional repository, following the sequence 3B.1 to 3B.7 in the brief,
   against the released extension API.

Until the private repository exists and licensing clearance is resolved, no
Professional functionality should be committed here and nothing should be
distributed as a combined commercial product.


## Resolution (Phase 3B, community side)

| Gap | Resolution |
|---|---|
| 1 UI contributions | Declarative views/forms/actions; menu workspace; host-rendered with theme tones (ADR-9) |
| 2 Storage | `storage.sql` provisions a private directory; the extension owns its database (ADR-10) |
| 3 Merge/delete events | `contact.deleted`, `organization.deleted`, `related_id` on `contact.merged`; `records.resolve` reports `merged_into` |
| 4 Publishing events | `events.publish` with declared, identifier-only types; not delivered to extensions yet |
| 5 CLI resources | `[[cli]]` resources routed through `cli.run`/`cli.schema`; `--demo` and `--database` may precede them |
| 6 Per-call latency | Measured ≈ 26 ms per request; long-lived mode still deferred |
| 7 Inbox/Radar | `signals.provide` contributes explained signals to the People Inbox (dismiss/snooze via `action.run`) |
| 8 Links to extension records | Kept extension-side; contact dossier panel; `records.resolve` for batch lookups; `projects.save` added |
| 9 Dev entitlements | `TIDECONTACTS_DEV_ENTITLEMENTS`, labelled "not a license"; per-capability entitlement |
| 10 Distribution | Unchanged: manual install, executable digest re-approval; no registry |
