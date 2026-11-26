package ext

import (
	"context"
	"encoding/json"
	"regexp"
)

const (
	professionalMCPProvider = "pro-sales"
	methodMCPRun            = "mcp.run"
)

// MCPRequest is a dedicated RPC. Actor comes from the locally selected MCP
// profile; neither Actor nor Action is accepted from MCP tool input.
type MCPRequest struct {
	Action string          `json:"action"`
	Actor  string          `json:"actor"`
	Input  json.RawMessage `json:"input"`
}

var mcpActor = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

var mcpCapabilities = map[string]string{
	"opportunities_search":    "pro.opportunities",
	"opportunities_get":       "pro.opportunities",
	"sales_pipeline_get":      "pro.sales-pipelines",
	"business_summary":        "pro.business-pulse",
	"business_briefing":       "pro.daily-briefing",
	"business_forecast":       "pro.forecasting",
	"business_attention":      "pro.daily-briefing",
	"reports_run":             "pro.report-builder",
	"automation_rules_list":   "pro.automation",
	"automation_rule_inspect": "pro.automation",
	"automation_simulate":     "pro.automation",
	"automation_runs_list":    "pro.automation",
	"playbooks_preview":       "pro.automation",
	"playbooks_start":         "pro.automation",
}

func mcpUnavailable() *ProtocolError {
	return perr(CodePermissionDenied, "Professional MCP action is not available")
}

func (m *Manager) mcpAvailableLocked(ctx context.Context, action string) bool {
	feature := mcpCapabilities[action]
	r := m.recs[professionalMCPProvider]
	if feature == "" || r == nil || r.status != StatusEnabled || !r.m.MCP ||
		!r.effective().Has("mcp.register") || !r.effective().Has("storage.sql") || !r.m.Storage.SQL ||
		r.entry.ExecSHA256 == "" || r.entry.ExecSHA256 != r.execSHA || m.o.Entitlements == nil {
		return false
	}
	c, ok := m.reg.Get(feature)
	if !ok || !c.Available || c.Provider != professionalMCPProvider {
		return false
	}
	d, err := m.o.Entitlements.Check(ctx, feature)
	return err == nil && d.Allowed
}

// MCPActions inspects contributions without starting any extension process.
func (m *Manager) MCPActions(ctx context.Context) map[string]bool {
	if m == nil || m.Discover(ctx) != nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool)
	for action := range mcpCapabilities {
		if m.mcpAvailableLocked(ctx, action) {
			out[action] = true
		}
	}
	return out
}

// RunMCP refreshes local grants and executable approval before each call. The
// invocation uses a dedicated host callback session with no write methods.
func (m *Manager) RunMCP(ctx context.Context, req MCPRequest) (json.RawMessage, error) {
	if m == nil || !mcpActor.MatchString(req.Actor) || mcpCapabilities[req.Action] == "" || m.Discover(ctx) != nil {
		return nil, mcpUnavailable()
	}
	m.mu.Lock()
	available := m.mcpAvailableLocked(ctx, req.Action)
	m.mu.Unlock()
	if !available {
		return nil, mcpUnavailable()
	}
	return m.invoke(ctx, professionalMCPProvider, methodMCPRun, req)
}
