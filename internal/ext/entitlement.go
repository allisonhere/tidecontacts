package ext

import (
	"context"
	"strings"
)

// EntitlementDecision is the answer to "may this capability be used?".
type EntitlementDecision struct {
	Capability string `json:"capability"`
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason,omitempty"`
	// Source names the provider that decided, such as "community".
	Source string `json:"source"`
	// ExpiresAt is RFC 3339 for time-limited entitlements; empty means none.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// EntitlementProvider decides whether a capability may be used. Implementations
// must work offline. A decision error is treated as denial.
//
// Phase 3A ships only CommunityEntitlements. There is no license format, key,
// activation or network check here; a future provider (perpetual, subscription,
// trial or enterprise, validated offline) plugs in behind this interface and is
// consulted at the service boundary, not merely by hiding UI.
type EntitlementProvider interface {
	Check(ctx context.Context, capability string) (EntitlementDecision, error)
}

// CommunityEntitlements allows every capability outside the reserved commercial
// "pro." namespace and denies "pro." capabilities because no provider grants them.
type CommunityEntitlements struct{}

// Check implements EntitlementProvider.
func (CommunityEntitlements) Check(_ context.Context, capability string) (EntitlementDecision, error) {
	d := EntitlementDecision{Capability: capability, Allowed: true, Source: "community"}
	if strings.HasPrefix(capability, "pro.") {
		d.Allowed, d.Reason = false, "no entitlement provider grants this capability"
	}
	return d, nil
}
