# Extension API specification (version 1)

## 1. Package layout

An extension is a directory whose name equals its id:

```text
<extensions dir>/relationship-digest/
  extension.toml        manifest (required, ≤ 64 KiB, strict: unknown keys are errors)
  relationship-digest   executable named by `command`
```

The directory must not be a symlink. The executable is resolved with symlinks
followed and must remain inside the directory, be a regular file, be marked
executable, and be ≤ 256 MiB. It is run directly (never through a shell) with
no arguments, working directory = the extension directory.

## 2. Manifest (`extension.toml`)

| Key | Type | Required | Notes |
|---|---|---|---|
| `id` | string | yes | `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`, 3–48 chars; must equal the directory name |
| `name` | string | yes | ≤ 60 chars, no control characters |
| `version` | string | yes | `major.minor[.patch]` |
| `api` | int | yes | Extension API major version. This host: `1` |
| `description`, `author` | string | no | ≤ 400 / 80 chars |
| `command` | string | yes | Clean relative path inside the directory (no `..`, `\`, absolute) |
| `min_host` | string | no | Minimum extension-host version (this host: `0.4.0`) |
| `requires` | [string] | no | Capability ids that must be available |
| `optional` | [string] | no | Capability ids used if present |
| `permissions` | [string] | no | Requested permissions (§4) |
| `entitlement` | string | no | Capability id the entitlement provider must allow; re-checked on every host call |
| `provides` | [[table]] | no | `id`, `version ≥ 1`; id must start with `<extension id>.` or `pro.` |
| `commands` | [[table]] | no | `id`, `title`, `description`; ≤ 16; needs `commands.register` |
| `panels` | [[table]] | no | `id`, `title`, `slot`; ≤ 4; needs `ui.register`; slot ∈ {`contact.dossier`} |
| `events.subscribe` | [string] | no | Event types (§6); needs `events.subscribe` and the entity's read permission |
| `config` | [[table]] | no | `key`, `label`, `type` (`string`,`integer`,`boolean`,`enum`), `default`, `options`, `min`, `max`, `help`; ≤ 16 |
| `storage` | table | no | `required`, `max_kb` (≤ 1024, default 256); needs `storage.extension` |

Validation never executes anything. A valid manifest is a *request*, not a
proof of trust or authorization (see [security.md](security.md)).

Complete example: [`examples/extensions/relationship-digest/extension.toml`](../../examples/extensions/relationship-digest/extension.toml).

## 3. Capabilities

A capability is a discoverable feature with a provider and an integer version.

| Id | Provider | Notes |
|---|---|---|
| `core.contacts`, `core.organizations`, `core.relationships`, `core.activities`, `core.tasks`, `core.projects`, `core.pipelines`, `core.insights`, `core.events` | core | Always available. Version 1. |
| `<extension-id>.<name>` | that extension | Available only while the extension is enabled and its requirements hold |
| `pro.*` | reserved | Reserved for future commercial extensions. Always unavailable under the Community entitlement provider. **Nothing in `pro.*` is implemented in this repository.** |

Rules enforced by the registry: `core.*` can never be provided by an extension;
a capability id has exactly one provider (second claimant is `conflict`);
requirements resolve to a fixed point, so a chain of dependencies settles and a
cycle never becomes available; disabled providers' capabilities report
`available: false` with a reason. List them with `tidecontacts capabilities list`.

## 4. Permissions

Default deny. The host checks the permission on every call, at the service
boundary (`ext.Session`), before touching the repository.

| Permission | Enforcement | Grants |
|---|---|---|
| `contacts.read` / `contacts.write` | host-api | `contacts.list`, `contacts.get` / `contacts.save` for people |
| `organizations.read` / `organizations.write` | host-api | the same calls for organizations |
| `relationships.read` | host-api | `contacts.connections` (plus a contact/organization read grant) |
| `activities.read` / `activities.write` | host-api | `activities.list` / `activities.save` |
| `tasks.read` / `tasks.write` | host-api | `tasks.list` / `tasks.save` |
| `projects.read` | host-api | `projects.list` |
| `projects.write` | host-api | `projects.save` |
| `pipelines.read` | host-api | `pipelines.list` |
| `events.subscribe` | host-api | Receive subscribed events |
| `commands.register` | host-api | Palette/CLI commands are only offered if granted |
| `ui.register` | host-api | Panels are only rendered if granted |
| `storage.extension` | host-api | `storage.*` |
| `network.access` | **declared-only** | Recorded and shown. The host cannot stop a same-user process using the network. |
| `relationships.write`, `pipelines.write` | reserved | Valid names; no host API yet |

* Approval is explicit: `enable` shows the list; the CLI needs `--yes`.
* The user's grant is exactly the set requested at approval time. If an update
  requests more, the extension becomes `needs-approval` and is not executed.
  Requesting fewer never triggers a prompt.
* All writes go through the same repository used by the UI, so domain
  validation, directory read-only ownership and optimistic revision checks
  apply. A stale revision returns `conflict`.
* Read results for `contacts.get` are checked *after* the read against the
  contact's kind, and denied before the read if neither read permission is held,
  so error differences cannot be used to probe ids.

## 5. Wire protocol

Transport: the host starts the executable once per request. Messages are single
JSON objects, one per line (UTF-8, `\n`), on stdin (host → extension) and stdout
(extension → host). stderr is captured (first 8 KiB), sanitized and shown only
as a diagnostic. Every message has `"api": 1` and a `type`.

```text
host → ext   {"api":1,"type":"request","request_id":"<hex>","method":"command.run","data":{…}}
ext  → host  {"api":1,"type":"host_call","call_id":"1","method":"contacts.list","data":{…}}     (zero or more)
host → ext   {"api":1,"type":"host_response","call_id":"1","ok":true,"data":…}
             or {"api":1,"type":"host_response","call_id":"1","ok":false,"error":{"code":"permission_denied","message":"…"}}
ext  → host  {"api":1,"type":"response","request_id":"<same>","ok":true,"data":{…}}            (exactly one, last)
             or {"api":1,"type":"response","request_id":"<same>","ok":false,"error":{"code":"…","message":"…"}}
```

Host limits (not configurable by extensions): message ≤ 1 MiB; total output ≤ 8 MiB;
≤ 2000 host calls per request; timeout 10 s by default (hard cap 30 s); ≤ 2
concurrent requests per extension; ≤ 16 command arguments. On timeout,
cancellation, malformed JSON, wrong `api`, wrong `request_id`, an unknown message
type or any limit breach the host kills the process (and its process group on
Unix), reaps it, records a failure and carries on. A `response` with `ok:false`
is a clean application error and does not count as a crash.

Environment: `PATH=/usr/bin:/bin`, `LANG=C.UTF-8`, `TIDECONTACTS_EXTENSION_ID`,
`TIDECONTACTS_EXTENSION_API`. Nothing else is inherited; no `HOME`, no secrets.

### Host → extension methods

| Method | `data` | Expected `response.data` |
|---|---|---|
| `activate` | `{host_api, host_version}` | `{}` — called at enable and at startup; failure disables/fails the extension |
| `deactivate` | `{}` | `{}` — best effort, 3 s |
| `health` | `{}` | `{"ok":bool,"message":string}` |
| `command.run` | `{command, args:{string:string}}` | Content (below) |
| `panel.render` | `{panel, entity_id}` | Content |
| `event.deliver` | `{event: Event}` | `{}` |

### Extension → host calls

`config.get`, `contacts.list {query, kind, limit}`, `contacts.get {id}`,
`contacts.connections {id}`, `contacts.save {Contact}`, `activities.list
{entity_id, query, before, limit}`, `activities.save {Activity}`, `tasks.list
{view, entity_id, limit}`, `tasks.save {Task}`, `projects.list {status}`,
`pipelines.list`, `storage.get|put|delete|keys {key, value}`. Arguments are
decoded strictly: unknown fields are `invalid_input`. Record shapes are the
domain types used by the CLI (`tidecontacts schema`). Lists are capped (default
100, maximum 500). Error codes: `permission_denied`, `unknown_method`,
`invalid_input`, `not_found`, `conflict`, `entitlement_required`,
`quota_exceeded`, `internal`.

### Content (host-rendered UI)

```json
{"title":"Relationship digest","lines":[{"label":"People","text":"12","level":"info"}]}
```

`level` ∈ `info` (default), `ok`, `warn`. The host bounds (≤ 40 lines; label ≤ 24,
text ≤ 160, title ≤ 60 runes), strips ANSI/OSC/control characters and
bidirectional overrides, and renders with the active theme. There is no way to
emit styling, positioning or key handling.

## 6. Events

Envelope (version 1):

```json
{"id":"a1b2…","type":"task.completed","schema_version":1,"time":"2026-11-25T18:04:05.123Z",
 "entity_type":"task","entity_id":"<uuid>","revision":3,"source":"ui","correlation_id":""}
```

Events carry identifiers only: never names, addresses, notes or message content.
`source` is `ui`, `cli`, `import` or `extension:<id>`. `correlation_id` groups a
multi-record write such as an import batch.

| Type | Emitted | Needs read permission |
|---|---|---|
| `contact.created/updated/archived/merged` | yes | `contacts.read` |
| `organization.created/updated` | yes | `organizations.read` |
| `activity.created/updated/deleted` | yes | `activities.read` |
| `task.created/updated/completed/reopened` | yes | `tasks.read` |
| `project.created/updated` | yes | `projects.read` |
| `pipeline.created/updated`, `pipeline.item_created/item_moved` | yes | `pipelines.read` |
| `project.status_changed`, `followup.due`, `reminder.due` | **reserved, not emitted** | |

Semantics:

* Emitted **only after the write returned without error** (after commit). A
  stale revision, a validation failure or a rolled-back batch emits nothing.
* "Created" means the caller supplied revision 0 (or no id, for pipelines and
  items); everything else is "updated". Demo seeding emits nothing.
* **Best-effort, in-memory delivery.** Publishing never blocks. Each extension
  has a queue of 64; a full queue drops the new event and increments
  `events_dropped` (visible in health). Nothing is replayed after a restart.
  There is no durable outbox; an extension that must not miss changes should
  reconcile by reading state.
* Events whose source is an extension are never delivered to extensions, so
  extensions cannot trigger each other in a loop.
* A failing delivery counts toward the failure breaker; cancellation stops
  delivery immediately at shutdown.

## 7. Storage

Each extension has a private key-value store at `<data>/extension-data/<id>/store.json`,
reachable only through `storage.*` host calls (permission `storage.extension`).
Keys `[a-z][a-z0-9_]{0,39}`, values ≤ 64 KiB, ≤ 256 keys, total ≤ `storage.max_kb`
(≤ 1 MiB). The path is derived from the validated id, so one extension cannot
name another's data, and the file records its owner. Writes are atomic
(temp file + rename). Extensions reference core records by UUID only; there are
no foreign keys into the core database, so a tombstoned or merged contact can
leave a dangling reference that the extension must tolerate.

No tables are added to the SQLite database, and extensions cannot modify core
tables. Schema migrations for extension data are the extension's own concern
(store a version key); the host does not run extension migrations in Phase 3A.

## 8. Entitlements

```go
type EntitlementProvider interface {
    Check(ctx context.Context, capability string) (EntitlementDecision, error)
}
```

`CommunityEntitlements` allows everything except `pro.*`. A decision error is a
denial. Checks happen when an extension is resolved (`entitlement-required`) and
again on **every host call** for extensions that declare `entitlement`, so the
check is at the service boundary and not only in the UI. There is no license
format, key, activation, expiry enforcement, hardware binding or network call in
this repository; a future provider implements the interface offline. Tests use
a small in-test provider.

## 9. Rich contributions (added after Phase 3A; still API version 1, additive)

These let an extension provide a full workspace, JSON CLI resources and Inbox
signals without any new rendering or database code in the host. All are optional,
need an explicit permission, and are invisible unless the extension is enabled.

### 9.1 Workspaces and declarative views

Manifest: `[[workspaces]] id, title` (needs `ui.register`; ≤ 2). The menu and
palette list a workspace only while its extension is enabled.

The host calls `view.render` `{workspace, tab, item, query, state, width, narrow}`
and the extension answers with a **View document**: `title`, `badge`, `tabs`,
`tab`, `heading`, `subheading`, `tiles` (label, value, note, tone), one of
`board` (columns of cards: title, subtitle, amount, progress 0–100, left/right
meta, badge) or `list` (rows of cells), `sections` (labelled lines, optionally
with a `target`), `notices`, `actions` (single-key bindings), `open` (action run by
Enter), `searchable`, `empty`, `dossier`. Tones are `ok warn danger info accent
muted` and map to the active theme; extensions cannot set colors, positions or
styles. Collections are bounded (e.g. ≤ 12 columns, ≤ 200 cards per column, ≤ 500
rows) and every string is sanitized.

Keys: the host owns `esc q h j k l ↑↓←→ tab [ ] / ? : space enter ctrl+*` and
paging keys; an extension may bind a single printable ASCII character otherwise
(duplicates and reserved keys are dropped). `action.run` `{workspace, action, tab,
item, target, state, args}` answers with an **ActionResult**: `errors`, `notice`,
`refresh`, `state`, `form`, `confirm`, `open` (a `target` with kind `ext`,
`contact`, `organization`, `task`, `activity` or `project`), `close`. Destructive
actions declare `confirm` and the host asks first. Following a core record
leaves the workspace (its place is remembered) and opens the record with the
existing back-navigation.

`form.get` / `form.submit` use host form components: field types `text`,
`multiline`, `select`, `contact`, `contacts`; the extension validates and answers
with per-field `errors`. The host enforces only `required`.

### 9.2 CLI resources

Manifest: `[[cli]] resource, description, actions` (needs `cli.register`; names of
built-in resources are reserved). `tidecontacts RESOURCE …` (resource first) sends
`cli.run` `{resource, argv, stdin}` — `stdin` only for `--input -` (≤ 900 KiB; pass
larger files by path and the extension reads them) — and receives `{data, count,
etag, possibly_truncated, warnings}`; the host writes the standard envelope.
`schema RESOURCE` uses `cli.schema`. Error codes map to exit codes: `usage` 2,
`invalid_input` 3, `not_found` 4, `conflict` 5, `permission_denied` /
`entitlement_required` 6, anything else 1.

### 9.3 Signals

Manifest: `signals = true` (needs `signals.provide`). `signals.list` returns
`{key, type, title, severity (info|warn|danger), reason, facts[], date, action,
target}`. They appear in the People Inbox under "<Extension> attention", and the
detail pane shows reason, facts and the suggested step. `x` calls `signal.dismiss`
and `z` calls `signal.snooze {until}` through `action.run` on the extension's first
workspace. The host does not score or order them.

### 9.4 Host calls added

* `records.resolve {kind, ids[≤500]}` → minimal projections of contacts,
  organizations, tasks, activities and projects (needs the matching read
  permission): name, status, due, archived/deleted, `merged_into`, or
  `missing`. Wrong-kind lookups report `missing` rather than disclosing the record.
* `entitlement.check {capability}` → the provider's decision.
* `events.publish {type, entity_id, revision, correlation_id}` (needs
  `events.publish`) for types declared in `[[published_events]] type, entity_type`.
  Identifiers only; extra fields are rejected; the host stamps
  `source: extension:<id>`; ≤ 1000 per request. Extension-sourced events are not
  delivered to extensions. Core event types cannot be published.
* Host calls per request: 2000 (was 200). The request timeout is `timeout_seconds`
  (default 10, hard cap 30).

### 9.5 SQL data directory

`[storage] sql = true` with `storage.sql` makes the host create
`<data>/extension-data/<id>/` (0700) and pass it as `TIDECONTACTS_EXTENSION_DATA`.
The extension keeps its own SQLite database there and runs its own versioned
migrations. The host cannot restrict what is inside (same-user trust, see
[security.md](security.md)); disabling never deletes it and `purge-data` deletes the
directory only for a disabled extension and never one holding a manifest.

### 9.6 Entitlements per capability, and development entitlements

Each `pro.*` capability an extension provides is entitled independently: an
extension holding a partial entitlement is enabled and exactly the entitled
capabilities are available (`entitlement.check` lets it gate features itself).
`entitlement` in the manifest still gates the extension as a whole.

`TIDECONTACTS_DEV_ENTITLEMENTS` (`*` or a comma list of `pro.` ids) enables a
development provider. It is **not a license**: no keys, signatures, expiry or
verification; anyone can set it; every decision and every management surface says
so. A malformed value grants nothing.

### 9.7 Events added

`contact.deleted` and `organization.deleted` (when a save marks the record
deleted) and a `related_id` on `contact.merged` naming the merged-away record,
which is also marked deleted with `merged_into` on the survivor.

## 10. Panel views and shared attention (host 0.4.0, API v1)

An extension using any of these additions should set `min_host = "0.4.0"`.
Extensions targeting the earlier host still work. No Community schema change.

### 10.1 Panels

Views may supply `panels` (≤ 16) instead of a board or list. Each panel has a
stable `id`, `title`, optional `short` label, `count`, `empty`, `full` (span the
grid), optional `open` target and up to 12 `blocks`. The host selects the column
count and clips each panel to the viewport; the extension supplies no styling.

| Block `kind` | Content |
|---|---|
| `lines` | Up to 200 labeled rows, with optional mark, tone and drill-down target |
| `bars`, `columns`, `stack`, `spark`, `trend` | Up to 120 data points; horizontal, vertical, stacked, sparkline and braille/ASCII line charts |
| `progress`, `bullet` | First datum compared with an explicit `goal` |
| `heat` | Bounded rows and column labels; a null cell is unknown |
| `text` | Wrapped explanatory text |

Chart values are integers for geometry only; supply the user-visible value,
units and currency in the datum's text. Mark unknown observations as `missing`
instead of encoding a zero. Lines and data points can carry a distinct `id`
and `target`. `target.workspace`, when set on an extension target, must name
another workspace of that same extension. Esc restores selection and filter
state after following an extension or Community record link.

`Tab`/`Shift+Tab` move panel focus, `[`/`]` move view tabs, arrows or `j/k`
select, and Enter follows a target. The command palette remains available.
`form.submit` receives the originating view's optional `state` alongside the
form's `args`, `values` and `refs`.

### 10.2 Shared attention

* `attention.list {}` returns the existing open People Inbox items (key, kind,
  reason, due date, urgency and record references). Needs `attention.read`,
  `contacts.read` and `tasks.read`; organization entries also need
  `organizations.read`.
* `attention.act {key, action, until}` uses the same Inbox review, dismissal and
  snooze state as Community. Needs `attention.write`. It does not edit records.
* `contacts.engagement {ids}` returns recorded interactions, configured
  follow-up, dates and task context for up to 200 contacts. Needs
  `activities.read`, `tasks.read` and the relevant contact/organization read
  permission. Task detail is a bounded sample, not a complete list.
* `tasks.complete {id}` completes a Community task using its normal recurrence
  and event behavior. Needs `tasks.write`.

### 10.3 CLI output

A manifest can register up to 16 CLI resources. Successful extension CLI
responses use the versioned JSON envelope unless the caller explicitly
passes `--raw` and the extension returns `output`. That string is written
without the envelope; terminal control characters are stripped, except LF, CR
and TAB used by text formats. Errors still use the JSON error envelope.
