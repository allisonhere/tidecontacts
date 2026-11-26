package app

import (
	"context"
	"time"
)

// MCPProfile is a user-owned grant, selected by the local stdio launcher and
// never by a client-supplied protocol name or request parameter.
type MCPProfile struct {
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Scopes   []string `json:"scopes"`
	Revision int64    `json:"revision"`
}

type MCPAudit struct {
	Sequence int64     `json:"sequence"`
	At       time.Time `json:"at"`
	Profile  string    `json:"profile"`
	Tool     string    `json:"tool"`
	Outcome  string    `json:"outcome"`
	Write    bool      `json:"write"`
}

// MCPState keeps administration and permission checks independent of the SDK.
// Audit entries contain operation names and outcomes, never arguments/results.
type MCPState interface {
	MCPProfile(context.Context, string) (MCPProfile, error)
	MCPProfiles(context.Context) ([]MCPProfile, error)
	SaveMCPProfile(context.Context, MCPProfile) (MCPProfile, error)
	BeginMCPAudit(ctx context.Context, profile, tool string, write bool) (int64, error)
	FinishMCPAudit(ctx context.Context, sequence int64, outcome string) error
	MCPAuditPage(ctx context.Context, before int64, limit int) ([]MCPAudit, error)
}
