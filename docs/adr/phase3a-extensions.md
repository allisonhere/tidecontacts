# Architectural decision records — Phase 3A extension architecture

Status of all records: **accepted** for Phase 3A, revisitable.

## ADR-1: Extensions are subprocesses speaking JSON lines (API v1)

*Context.* Optional, independently distributed code must not crash or block the
host, and the project already has a working subprocess protocol (TideMail
plugin: one JSON request on stdin, one JSON response on stdout, `api: 1`,
size and time limits).

*Decision.* Reuse that shape: one process per request, newline-delimited JSON,
`api` and `request_id` on every message. Add `host_call` / `host_response`
messages on the same pipes so an extension can use the host API during a request.

*Rejected.*
* **Go `plugin` package** — Linux/macOS only, requires identical toolchain and
  dependency versions, cannot be unloaded, and a plugin crash or data race
  takes down the host. No compelling reason to use it.
* **Long-lived JSON-RPC server per extension** — gives faster calls and
  in-memory state, but needs process supervision, restart/backoff, framing for
  concurrent requests and a stdout-clean discipline. Per-call processes make
  timeouts, cancellation and cleanup trivial and keep the contract tiny.
  Revisit if call latency matters (see Phase 3B).
* **In-process scripting (Lua/JS/WASM)** — extra dependency, a second runtime to
  secure, and weaker portability for authors. WASM remains an option for a
  future sandboxed runtime.
* **Network service/HTTP** — out of scope and would add a listener.

*Consequences.* ~ms of process start per call; no cross-call memory; events
cost one spawn each. Any language works.

## ADR-2: Authorize at the host API; don't pretend to sandbox

*Context.* A subprocess as the same user has OS access regardless of the
manifest.

*Decision.* Permissions are default-deny, per method, enforced in
`ext.Session` before the repository is touched; the grant is approved ∩
requested; growth or a changed executable forces re-approval. Each permission
carries an `enforcement` label (`host-api`, `declared-only`, `reserved`) so the
UI and CLI never overstate protection (`network.access` is `declared-only`).

*Rejected.* Trusting manifests; an OS sandbox in this phase (platform-specific,
large, and orthogonal to getting the API contract right).

## ADR-3: Events come from a store wrapper, delivered in-process, best-effort

*Decision.* `observe.Store` embeds `*sqlite.Store`, overrides the write methods
and publishes after success. The bus is in-memory with bounded non-blocking
queues; drops are counted. Events hold ids only.

*Rejected.*
* **Events inside the storage layer / SQL triggers** — couples persistence to
  delivery and would fire for directory refreshes and imports with no way to
  attribute source.
* **Transactional outbox / durable queue** — the right answer for automation and
  sync later, but it needs a schema migration, replay and retention policy;
  Phase 3A promises only best-effort delivery and documents it.
* **Broker** — explicitly out of scope.

*Consequences.* A CLI write does not reach a running TUI's extensions; folder
refresh and TideMail sync emit nothing; a crash loses queued events.

## ADR-4: Extension data is a private key-value store outside the core database

*Decision.* `<data>/extension-data/<id>/store.json`, owner-stamped, quota-limited,
reachable only via `storage.*`. No tables are added to SQLite.

*Rejected.*
* **Extension tables in the core database** — extensions would share the file
  with customer data, migrations from third parties would run against it, and
  "no empty professional tables" would be violated.
* **A SQLite file per extension handed to the extension** — gives richer queries,
  but it is a raw handle (same-user file access makes it unrestricted anyway) and
  needs a migration runner. Can be added later as an optional `storage.sql`
  capability without changing this contract.

*Consequences.* Simple backup (copy the directory), clean disable (nothing is
touched), explicit purge; no foreign keys into core data, so extensions must
tolerate dangling UUIDs.

## ADR-5: Entitlements are an interface with a Community provider only

*Decision.* `EntitlementProvider.Check` plus a decision type; the Community
provider allows everything outside `pro.*`. Checks run when resolving an
extension and on every host call for extensions that declare an entitlement.

*Rejected.* Any license format, key, trial logic, activation, machine
fingerprint, phone-home or DRM; a fake "demo license" that could be mistaken for
enforcement. Tests use a tiny in-test provider.

## ADR-6: Declarative, host-rendered UI contributions

*Decision.* Extensions return `Content` (title and labelled lines with a level).
The host renders it in its own modal or as rows under the dossier, with the
current theme, the host's keys and bounded size. One slot (`contact.dossier`) and
palette commands exist; extensions cannot bind keys.

*Rejected.* Extension-rendered ANSI (injection, theming and width bugs, no
accessibility), navigation entries or dashboard panels now (they would add
permanent empty chrome to Community; the validator rejects unsupported slots
until the host implements them).

## ADR-7: No new application interface hierarchy

*Decision.* Keep `app.CRM`/`app.Contacts` as the contract; capabilities are a
registry of names plus permissions mapped onto existing method groups.

*Rejected.* Splitting `app.CRM` into several interfaces now: no consumer needs
the narrower types yet, every existing caller would churn, and `ext.Session`
already provides the narrow, permissioned view. Revisit when a second frontend
needs a subset.

## ADR-8: Approval state is a plain file beside the database

*Decision.* `extensions.json` stores grants, executable digests and settings.

*Rejected.* Signing it (no key management story without accounts) or storing it
in SQLite (the CLI would need the database open to list extensions, and the
file is already within the same-user trust boundary).
*Consequence.* A same-user attacker can edit it; documented in the security
model.
