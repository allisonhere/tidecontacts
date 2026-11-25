package ext

import (
	"context"
	"fmt"
	"os"
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

// DevEntitlementsEnv names the environment variable that enables development
// entitlements. It exists so Professional extensions can be built and tested
// before any real licensing exists.
const DevEntitlementsEnv = "TIDECONTACTS_DEV_ENTITLEMENTS"

// DevEntitlements grants a listed set of "pro." capabilities for development
// and tests. It is NOT a license mechanism: it has no keys, expiry, signature
// or verification, anyone can set the variable, and it must never be mistaken
// for enforcement. Every decision it makes says so.
type DevEntitlements struct {
	all     bool
	allowed map[string]bool
}

// ParseDevEntitlements reads "*" (every pro. capability) or a comma-separated
// list of capability ids.
func ParseDevEntitlements(spec string) (DevEntitlements, error) {
	d := DevEntitlements{allowed: map[string]bool{}}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case part == "*":
			d.all = true
		case capPattern.MatchString(part) && strings.HasPrefix(part, "pro."):
			d.allowed[part] = true
		default:
			return d, fmt.Errorf("%s: %q is not '*' or a pro. capability id", DevEntitlementsEnv, part)
		}
	}
	return d, nil
}

// Check implements EntitlementProvider: Community rules, plus the listed pro. capabilities.
func (d DevEntitlements) Check(ctx context.Context, capability string) (EntitlementDecision, error) {
	base, _ := CommunityEntitlements{}.Check(ctx, capability)
	if base.Allowed || !strings.HasPrefix(capability, "pro.") {
		return base, nil
	}
	if d.all || d.allowed[capability] {
		return EntitlementDecision{Capability: capability, Allowed: true, Source: "development", Reason: "development entitlement; not a license"}, nil
	}
	return base, nil
}

// ProviderFromEnv returns the entitlement provider for this process and, when
// development entitlements are active, a notice every management surface shows.
func ProviderFromEnv() (EntitlementProvider, string) {
	spec := os.Getenv(DevEntitlementsEnv)
	if spec == "" {
		return CommunityEntitlements{}, ""
	}
	d, err := ParseDevEntitlements(spec)
	if err != nil {
		return CommunityEntitlements{}, "ignored " + err.Error()
	}
	return d, "development entitlements are active (" + DevEntitlementsEnv + "); this is not a license"
}
