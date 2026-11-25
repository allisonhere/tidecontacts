# Changelog

## Unreleased

- Extension host, Phase 3B: extensions may contribute a menu workspace with declarative views and forms, JSON CLI resources, People Inbox signals, declared identifier-only events and a private SQL data directory; `records.resolve`, `entitlement.check` and `projects.save` host calls; `contact.deleted`, `organization.deleted` and a `related_id` on `contact.merged`; per-capability entitlements and an explicit development entitlement provider (`TIDECONTACTS_DEV_ENTITLEMENTS`, not a licence). Fixes found by running a real extension: extension activities now use local provenance, and the host call budget is the documented 2 000. See `docs/extensions/specification.md` §9.

- Added optional extension infrastructure (Phase 3A): validated `extension.toml` manifests, subprocess extensions over a versioned JSON-lines protocol, default-deny permissions enforced at the host API, a capability registry, typed best-effort domain events, extension-owned storage, an entitlement interface (Community provider only), a host-rendered management modal and palette entries, and `extensions` / `capabilities` JSON CLI commands (new exit code 6). Includes the read-only *Relationship Digest* example. Nothing changes without extensions installed; the database schema stays at 4. See `docs/extensions/`.

- Simplified contact editing with Name, Email, and Phone on the first screen. Enter advances through fields, Ctrl+S or F2 saves, and the seven section tabs support arrow navigation.
- Made pane and menu navigation consistent. Highlighting a workspace previews its records, Tab or Right enters the list, and numbered and action shortcuts work while the menu has focus.
- Improved contact browsing with email addresses in the list, clearer search and return hints, and record metadata in Context.
- Improved connected vCard folders. Extensionless vCards and address-only contacts are accepted, an empty folder is reported during initial connection, and refresh results include unchanged contacts.
