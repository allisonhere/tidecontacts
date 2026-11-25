# Extension architecture (Phase 3A)

TideContacts can be extended by optional, separately installed programs. This
page is the overview; the other pages are the reference:

| Page | Contents |
|---|---|
| [specification.md](specification.md) | Manifest format, wire protocol, capabilities, permissions, events, storage, entitlements |
| [developing.md](developing.md) | Writing, installing and testing an extension; a complete working example |
| [security.md](security.md) | Threat model, trust model, mitigations and known limitations |
| [../adr/phase3a-extensions.md](../adr/phase3a-extensions.md) | Architectural decision records and rejected alternatives |
| [licensing-boundary.md](licensing-boundary.md) | The proposed Community/Professional boundary, pending legal review |

**Nothing here changes Community behaviour.** With no extensions installed,
TideContacts runs exactly as before: no processes are started, no tables are
added, the palette gains one entry ("Manage extensions…"), and the existing
database opens unchanged (the schema stays at version 4).

## Layers

```text
 Presentation        TUI (host-rendered modal, palette, dossier rows)
                     JSON CLI (extensions …, capabilities list)
                     future: desktop / web / local API (not implemented)
        │                     │
        ▼                     ▼
 ┌──────────────────────────────────────────────────────────────┐
 │ internal/ext   Manager: discovery · approval · lifecycle ·    │
 │                health · event routing · config                │
 │   Registry (capabilities)   EntitlementProvider (interface)   │
 │   Session  ← the only door extensions get; checks permissions │
 │   Storage  (private key-value, one directory per extension)   │
 │   Bus      (typed events, bounded, best-effort)               │
 └───────┬───────────────────────────────┬──────────────────────┘
         │ app.CRM (existing interface)   │ one request per process,
         ▼                                │ JSON lines over stdin/stdout
 internal/ext/observe  → publishes events ▼
 internal/storage/sqlite (unchanged)   extension program (any language)
```

* **`app.CRM` is unchanged.** The existing application interface already splits
  cleanly into capabilities (contacts, activities, tasks, projects, pipelines,
  insights); the extension host maps permissions onto those groups instead of
  introducing a second interface hierarchy. New consumers (a future HTTP server
  or desktop frontend) can reuse `ext.Session` or `app.CRM` directly.
* **Events are emitted by a wrapper, not the store.** `observe.Store` embeds the
  SQLite store and publishes an event only after a write returns without error.
  Nothing in `internal/storage/sqlite` changed.
* **Extensions are subprocesses.** The protocol extends the TideMail plugin
  protocol (one JSON request, one JSON response, API version 1) with host calls
  on the same pipes. No Go `plugin` package, no shared memory, no database handle.

## Lifecycle

```text
 discovered ──enable (user approves permissions)──► activating ──ok──► enabled
     ▲                                                   │              │
     │                                                fail│     5 straight failures
   disable ◄──────────────────────────────────────────────┘              ▼
     ▲                                                                 failed ──re-enable──►
     └── permissions grew / executable changed ──► needs-approval
```

| Status | Meaning |
|---|---|
| `disabled` | Valid, not enabled. Never executed. |
| `enabled` | Approved and active. Commands, panels and events are live. |
| `failed` | Five consecutive crashes/timeouts; the host stopped calling it. Re-enable to retry. |
| `needs-approval` | Enabled earlier, but the manifest now requests permissions that were not approved, or the executable's SHA-256 changed. Not executed until re-approved. |
| `unsatisfied` | A required capability is not available. |
| `entitlement-required` | The entitlement provider denied a capability it declares. |
| `conflict` | It tried to provide a capability another extension (or the core) owns. |
| `incompatible` | Needs a different extension API or a newer host. |
| `invalid` | Malformed manifest, duplicate id, directory/id mismatch, or unusable executable. |

Discovery (`internal/ext.Manager.Discover`) only reads manifests and hashes
executables; it never runs extension code. Activation runs after startup in a
background goroutine, so a slow or hung extension cannot delay or block the TUI.
At shutdown `Manager.Close` cancels delivery, kills in-flight processes (the
whole process group on Unix) and waits for them before the database closes.

## Where things live

| What | Where |
|---|---|
| Extension programs | `<config dir>/tidecontacts/extensions/<id>/` (override: `TIDECONTACTS_EXTENSIONS_DIR`, absolute) |
| Approvals, granted permissions, executable digests, settings | `<data dir>/tidecontacts/extensions.json` |
| Extension-owned data | `<data dir>/tidecontacts/extension-data/<id>/store.json` |
| The contacts database | unchanged; extensions never receive it |

Back up an installation by copying `tidecontacts.db`, `extensions.json` and the
`extension-data/` directory together. Disabling an extension never deletes its
data; `tidecontacts extensions purge-data <id> --yes` does, and only for a
disabled extension. Uninstalling is deleting the extension's program directory.

## Using it

TUI: open the palette (`:` or `Ctrl+K`) and choose **Manage extensions…**.
`↑↓` select, `Enter` details (permissions, health, sanitized diagnostics), `e`
enable/disable (enabling always shows the permission list first), `h` health,
`Esc` back/close. Enabled extensions add their commands to the palette as
"Extension name: Command"; extensions cannot bind keys.

CLI (JSON, same permission and validation boundaries):

```sh
tidecontacts extensions list
tidecontacts extensions inspect relationship-digest
tidecontacts extensions permissions relationship-digest
tidecontacts extensions enable relationship-digest --yes   # --yes = you approved the permissions
tidecontacts extensions commands
tidecontacts extensions run relationship-digest digest
tidecontacts extensions config relationship-digest --set window_days=14
tidecontacts extensions health relationship-digest
tidecontacts extensions disable relationship-digest
tidecontacts extensions purge-data relationship-digest --yes
tidecontacts capabilities list
```

Exit codes follow the CLI: 0 success, 1 runtime, 2 usage, 3 invalid input,
4 not found, 5 conflict, and new in this phase **6 denied** (approval,
permission or entitlement missing; the JSON error code says which).

## Try the demonstration extension

```sh
make build extension-demo                       # builds bin/extensions/relationship-digest
export TIDECONTACTS_EXTENSIONS_DIR=$PWD/bin/extensions
./bin/tidecontacts --demo extensions permissions relationship-digest
./bin/tidecontacts --demo extensions enable relationship-digest --yes
./bin/tidecontacts --demo extensions run relationship-digest digest
./bin/tidecontacts --demo                       # Ctrl+K → "Relationship Digest: …";
                                                # the contact Dossier view shows its "Digest" panel
```

`--demo` keeps approvals and extension data with the fictional demo profile, so
nothing touches your real data. To install permanently, copy the directory to
`~/.config/tidecontacts/extensions/` instead and drop `--demo`.

## Compatibility guarantees

* The extension API is versioned (`api = 1`). Within a major version the host
  only adds optional fields, methods and event types; it never removes or
  changes them. A host that does not speak a manifest's `api` marks it
  `incompatible` rather than guessing.
* Event envelopes carry `schema_version`; new fields are additive.
* The core capability ids (`core.*`) and permission ids are stable.
* Existing CLI commands, envelopes, exit codes 0–5, the SQLite schema, the
  TideMail plugin protocol and import/export formats are unchanged.

## Known limitations (Phase 3A)

See [security.md](security.md#limitations) for security-relevant ones.

* One process per call; extensions cannot keep in-memory state between calls
  (use `storage.extension`). Event delivery spawns one process per event.
* Events are in-memory and best-effort. They are not emitted for connected
  folder/Google refreshes or TideMail activity sync, and not across processes
  (a CLI write does not notify a running TUI's extensions).
* `project.status_changed`, `followup.due` and `reminder.due` are defined but not
  yet emitted. `relationships.write`, `projects.write`, `pipelines.write` are
  defined but have no host API yet.
* Only one UI slot exists (`contact.dossier`). Navigation entries, dashboard
  panels, context menus, search providers and settings panels are reserved and
  rejected by the manifest validator until the host renders them.
* There is no extension installer, marketplace, signature verification or update
  mechanism, by design.
