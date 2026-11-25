# Proposed Community/Professional boundary

> **Status: proposal for legal review. Nothing here is legal advice, a license
> grant, or a statement that commercial distribution has been cleared.** Licensing
> is pending (see [COMMERCIAL-LICENSING.md](../../COMMERCIAL-LICENSING.md) and
> [the clearance checklist](../licensing-checklist.md)). Phase 3A changes no
> license, contributor agreement or notice, and implements no commercial
> functionality.

## What this repository contains

Only Community-side infrastructure and a non-commercial demonstration extension:
the manifest and protocol, capability registry, permissions, events, extension
storage, an entitlement *interface* with a Community provider that always says
"no" to `pro.*`, the CLI and TUI management surfaces, and
`relationship-digest`. It contains no sales opportunities, forecasting,
commercial reporting, lifecycle management, automation engine, license keys,
activation or payment code. The reserved `pro.*` capability namespace is a name
reservation, not an implementation.

## The boundary as designed

```text
 Community (this repository)              Optional external extension
 ──────────────────────────               ───────────────────────────
 domain, storage, CLI, TUI                 separate program / repository
 internal/ext (host, API v1)  ◄── JSON lines over stdin/stdout ──►  speaks API v1
 EntitlementProvider interface             may ship its own EntitlementProvider
                                           (not part of Community)
```

* The only contract is the public, versioned, documented protocol. Extensions
  cannot import `internal/` packages and receive no database handle.
* Community works without any extension and never requires a commercial
  component. Existing functionality is not gated.
* Entitlement checks sit at the service boundary, but Community ships no
  mechanism that restricts Community features.

## Open questions for counsel (not resolved here)

1. Whether a separate program communicating through a documented subprocess
   protocol is a separate work for AGPL-3.0 purposes. Separate repositories,
   dynamic loading, subprocesses and IPC **do not automatically** establish
   independence; this is a question of fact and law, and this design does not
   assume the answer.
2. Whether the protocol and manifest specification, and any shared data schemas
   (e.g. the domain JSON an extension receives), create obligations for
   extensions, and what notices and source offers are needed if an extension is
   distributed together with the Community binary.
3. The effect of the AGPL's network-interaction clause on a hosted offering
   ("TideContacts Cloud") that runs Community code and extensions.
4. Contributor permissions and any relicensing authority; clearance of Ripple,
   TideUI and TideMail terms; dependency notices.
5. Whether the demonstration extension, shipped under the repository license,
   should carry its own notice to avoid implying a license for other extensions.
6. How an eventual commercial entitlement provider may be distributed and
   whether it must be offered with corresponding source.

Until these are answered, treat any Professional distribution plan as
unapproved.
