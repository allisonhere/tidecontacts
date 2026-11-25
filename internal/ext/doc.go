// Package ext is the community-side extension infrastructure: manifest
// validation, a capability registry, granular permissions, typed domain events,
// an entitlement interface, extension-owned storage and a subprocess lifecycle
// manager.
//
// Nothing here depends on the terminal UI. The TUI, the JSON CLI and future
// frontends consume Manager. Extensions never receive a database handle; they
// call a permission-checked host API (Session) over a bounded JSON protocol.
//
// Permissions are API authorization, not operating-system sandboxing. See
// docs/extensions/security.md.
package ext

// APIVersion is the extension protocol major version this host speaks.
const APIVersion = 1

// HostVersion is compared against a manifest's min_host.
const HostVersion = "0.3.0"
