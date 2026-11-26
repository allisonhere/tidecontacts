package ext

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

const graphicalMethod = "graphical.read"
const graphicalCapability = "pro.graphical-read"
const graphicalProvider = "pro-sales"

type graphicalOperation struct {
	resource, action, detail, entitlement string
}

// Only these known read operations can become graphical routes. No raw command,
// stdin, paths, arbitrary extension names, mutations or approvals come from HTTP.
var graphicalOperations = map[string]graphicalOperation{
	"opportunities":   {"opportunities", "list", "get", "pro.opportunities"},
	"sales-pipelines": {"sales-pipelines", "list", "inspect", "pro.sales-pipelines"},
	"customers":       {"customers", "list", "", "pro.customer-lifecycle"},
	"products":        {"products", "list", "", "pro.products-services"},
	"pulse":           {"business", "summary", "", "pro.business-pulse"},
	"briefing":        {"business", "briefing", "", "pro.daily-briefing"},
	"forecast":        {"business", "forecast", "", "pro.forecasting"},
	"rescue":          {"business", "rescue", "", "pro.opportunity-rescue"},
	"watchtower":      {"business", "watchtower", "", "pro.customer-watchtower"},
	"concentration":   {"business", "concentration", "", "pro.revenue-intelligence"},
	"renewals":        {"analytics", "renewals", "", "pro.renewal-management"},
	"targets":         {"targets", "progress", "", "pro.business-targets"},
	"scenarios":       {"scenarios", "list", "get", "pro.revenue-simulator"},
	"replay":          {"replay", "show", "", "pro.business-replay"},
	"reports":         {"reports", "list", "run", "pro.report-builder"},
	"rules":           {"automation", "list", "show", "pro.automation"},
	"runs":            {"automation", "history", "trace", "pro.automation"},
	"approvals":       {"automation", "approvals", "", "pro.automation"},
	"playbooks":       {"playbooks", "list", "inspect", "pro.automation"},
	"schedules":       {"schedules", "list", "inspect", "pro.automation"},
}

type GraphicalQuery struct {
	Operation, ID, Period, Currency, Query, Date string
	Offset                                       int
}

type graphicalRequest struct {
	CLIRequest
	query GraphicalQuery
}

var graphicalCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func (q GraphicalQuery) cli() (CLIRequest, bool) {
	o, ok := graphicalOperations[q.Operation]
	if !ok || strings.HasPrefix(q.ID, "-") || len(q.ID) > 200 || len(q.Query) > 512 || len(q.Period) > 30 || len(q.Date) > 10 || q.Offset < 0 || q.Offset > 100000 || (q.Currency != "" && !graphicalCurrency.MatchString(q.Currency)) {
		return CLIRequest{}, false
	}
	a := []string{o.action}
	if q.ID != "" {
		if o.detail == "" {
			return CLIRequest{}, false
		}
		a = []string{o.detail, q.ID}
	}
	switch q.Operation {
	case "opportunities":
		if q.ID == "" {
			a = append(a, "--limit=50", "--offset="+strconv.Itoa(q.Offset))
			if q.Query != "" {
				a = append(a, "--query="+q.Query)
			}
		} else {
			a = append(a, "--history")
		}
	case "rules":
		if q.ID == "" {
			a = append(a, "--limit=50", "--offset="+strconv.Itoa(q.Offset))
		}
	case "runs", "approvals":
		if q.ID == "" {
			a = append(a, "--limit=50")
		}
	case "products":
		if q.Query != "" {
			a = append(a, "--query="+q.Query)
		}
	case "replay":
		a = append(a, "--date="+q.Date)
	}
	if q.Period != "" && (q.Operation == "pulse" || q.Operation == "forecast" || q.Operation == "concentration" || q.Operation == "reports" && q.ID != "") {
		a = append(a, "--period="+q.Period)
	}
	if q.Currency != "" && (o.resource == "business" || o.resource == "analytics" || o.resource == "replay" || o.resource == "opportunities" && q.ID == "" || q.Operation == "reports" && q.ID != "" || q.Operation == "scenarios" && q.ID != "") {
		a = append(a, "--currency="+q.Currency)
	}
	return CLIRequest{Resource: o.resource, Argv: a}, true
}

func (m *Manager) graphicalAllowedLocked(ctx context.Context, q GraphicalQuery) bool {
	o, ok := graphicalOperations[q.Operation]
	r := m.recs[graphicalProvider]
	if !ok || r == nil || r.status != StatusEnabled || !r.effective().Has("cli.register") || !r.effective().Has("storage.sql") || !r.m.Storage.SQL || r.entry.ExecSHA256 == "" || r.entry.ExecSHA256 != r.execSHA || m.o.Entitlements == nil {
		return false
	}
	for _, capID := range []string{graphicalCapability, o.entitlement} {
		c, ok := m.reg.Get(capID)
		if !ok || !c.Available || c.Provider != graphicalProvider {
			return false
		}
	}
	action := o.action
	if q.ID != "" {
		action = o.detail
	}
	if !slices.ContainsFunc(r.m.CLI, func(d CLIDecl) bool { return d.Resource == o.resource && slices.Contains(d.Actions, action) }) {
		return false
	}
	decision, err := m.o.Entitlements.Check(ctx, o.entitlement)
	return err == nil && decision.Allowed
}

func graphicalUnavailable() *ProtocolError {
	return perr(CodePermissionDenied, "Professional graphical operation is unavailable")
}

// GraphicalActions returns the currently authorized Professional read operations.
func (m *Manager) GraphicalActions(ctx context.Context) []string {
	out := []string{}
	if m == nil || m.Discover(ctx) != nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for operation := range graphicalOperations {
		if m.graphicalAllowedLocked(ctx, GraphicalQuery{Operation: operation}) {
			out = append(out, operation)
		}
	}
	slices.Sort(out)
	return out
}

// RunGraphicalRead uses the dedicated read dispatcher on the approved extension.
func (m *Manager) RunGraphicalRead(ctx context.Context, q GraphicalQuery) (CLIResponse, error) {
	req, ok := q.cli()
	if !ok {
		return CLIResponse{}, perr(CodeInvalid, "Invalid Professional read parameters")
	}
	if m == nil || m.Discover(ctx) != nil {
		return CLIResponse{}, graphicalUnavailable()
	}
	m.mu.Lock()
	allowed := m.graphicalAllowedLocked(ctx, q)
	m.mu.Unlock()
	if !allowed {
		return CLIResponse{}, graphicalUnavailable()
	}
	raw, err := m.invoke(ctx, graphicalProvider, graphicalMethod, graphicalRequest{CLIRequest: req, query: q})
	if err != nil {
		return CLIResponse{}, err
	}
	var result CLIResponse
	if err = json.Unmarshal(raw, &result); err != nil || len(result.Data) == 0 || result.Output != "" {
		return CLIResponse{}, perr(CodeInternal, "Invalid Professional response")
	}
	return result, nil
}
