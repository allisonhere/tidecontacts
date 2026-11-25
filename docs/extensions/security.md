# Security and trust model

## The one-paragraph summary

An extension is a program the user chose to install and run. TideContacts
**authorizes its use of the host API** (default deny, per-permission, enforced
on every call) and **bounds how it can misbehave toward the host** (time,
output, call counts, concurrency, process cleanup, text sanitizing). It does
**not sandbox the operating system**: a subprocess running as the same user can
still read and write that user's files (including the contacts database and
`extensions.json`), use the network and read the OS keychain. Manifest
permissions must never be presented as OS-level isolation.

## Trust model

| Party | Trusted to… |
|---|---|
| The user | decide what to install and approve |
| The host (`tidecontacts`) | enforce the API boundary, validate input, sanitize output |
| An extension | nothing. Its manifest is a *request*; its responses are untrusted input |
| An extension's author | nothing is verified: there is no signing, registry or provenance check |

## Threats and status

✓ mitigated by the host · ◐ partly · ✗ not mitigated (needs OS sandboxing or future work)

| Threat | Status | Notes |
|---|---|---|
| Malicious extension abusing the host API | ✓ | Default-deny permissions checked in `ext.Session` before the repository; writes pass the same validation, directory-ownership and revision checks as the UI |
| Malicious extension reading/writing files, the database or the keychain directly | ✗ | Same-user process. Needs bubblewrap/Landlock/seccomp (Linux), App Sandbox (macOS) or a separate user. The database is never handed over, but it is not hidden either |
| `network.access` | ✗ | Recorded and shown (`declared-only`); not enforceable without a sandbox. Data you grant read access to can be sent anywhere |
| Malformed or hostile manifest | ✓ | Strict TOML (unknown keys rejected), 64 KiB cap, UTF-8, control characters rejected, id/command/capability grammars, count limits, namespace rules |
| Oversized messages / output floods | ✓ | 1 MiB per message, 8 MiB per invocation, 8 KiB retained stderr, 2000 host calls, ≤ 500-row lists |
| Protocol confusion | ✓ | `api`, `type` and `request_id` checked; unknown types, wrong ids and mixed versions kill the call |
| Command injection | ✓ | The executable is run directly with no arguments and no shell; the host never builds a command string from extension data |
| Path traversal / symlink escape | ✓ | Directory name must equal a validated id; `command` is a clean relative path whose real (symlink-resolved) target must stay inside the directory; storage paths derive only from the validated id; storage keys are `[a-z0-9_]` |
| Executable swapped after approval | ◐ | SHA-256 recorded at approval; a change is `needs-approval`. The hash is checked at discovery, not at the instant of `exec`, so a process with write access to the directory can race it. Directory permissions are the user's responsibility |
| Secret disclosure to extensions | ◐ | The host API exposes no keychain, token or configuration secret; the environment is scrubbed (`PATH`, `LANG` and two ids only); logs and diagnostics contain ids and sanitized text, never payloads. An extension can still read files itself (✗ above) |
| Permission escalation | ◐ | Effective permissions = approved ∩ currently requested. A larger request or changed executable stops the extension until re-approval. `extensions.json` is **not** integrity-protected against a same-user attacker, who could edit it |
| Unbounded process execution | ◐ | Timeout (10 s default, 30 s cap), ≤ 2 concurrent calls per extension, process-group kill, reaping, 5-failure breaker, shutdown cancellation. **No CPU or memory limits** (no rlimits/cgroups) |
| Crashes and hangs | ✓ | Isolated in a subprocess; recorded in health; never panics the host; startup activation is asynchronous |
| Event flooding | ✓ | Non-blocking bounded queues (drop and count); events from extensions are not redelivered to extensions |
| Terminal escape injection | ✓ | All extension text passes `ext.Clean` (ANSI/OSC stripped, controls and bidi overrides removed, lengths capped) and is rendered by the host; extensions cannot emit styling |
| Unauthorized database mutation through the API | ✓ | No handle; only the listed calls; writes need `*.write` |
| Stale/concurrent updates | ✓ | Optimistic revisions apply; stale writes return `conflict` |
| Event content leakage | ✓ | Events hold ids and revisions only |
| Data loss when disabling | ✓ | Disable never deletes; purge is explicit, separate and refuses while enabled |
| Confused-deputy via entitlements | ◐ | Checked at resolution and every host call for extensions that declare one. No production licensing exists; do not treat the interface as enforcement |

## Added in Phase 3B

| Surface | Control |
|---|---|
| `storage.sql` | The host creates the data directory `0700` and tells **only** extensions holding the permission where it is; contents are not restricted (same-user trust) and `purge-data` refuses a directory holding a manifest |
| Declarative views | Every string is sanitised and bounded; keys cannot hijack reserved bindings; collections are capped; actions that declare `confirm` are confirmed by the host; core-record links use fixed kinds and token ids |
| `cli.register` | Built-in resource names are reserved; arguments are passed as data; stdin is size-limited |
| `signals.provide` | Capped, sanitised, ordered by the extension's own severity only; the host does not score |
| `events.publish` | Declared types only, identifier payloads only (extra fields rejected), host-stamped source, ≤ 1 000 per request, never delivered back to extensions |
| `records.resolve` | Read permission per kind; ≤ 500 ids; minimal projection; a wrong-kind id looks like a missing one |
| Development entitlements | Opt-in by environment variable, labelled in every decision and banner, malformed value grants nothing |

## Limitations

* No OS sandbox (see above). A future phase should add an optional sandbox
  launcher (Linux namespaces + seccomp/Landlock, macOS sandbox profile) and
  could then enforce `network.access`.
* No resource limits for CPU, memory or open files.
* No code signing, publisher identity, revocation or update channel.
* Approval state is a plain file in the user's data directory.
* Event delivery is in-process and best-effort; a restart loses queued events.
* `contacts.read` is all-or-nothing: no field-level or per-collection scopes.
* The TUI shows an extension's sanitized stderr tail as "diagnostic". It is
  truncated and cleaned, but it is still text the extension chose.

## Reporting

Treat extension infrastructure bugs like any security issue in the host:
report privately to the maintainers before publishing details.
