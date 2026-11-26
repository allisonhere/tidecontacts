package mcpserver

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/allisonhere/tidecontacts/internal/ext"
)

type proID struct {
	ID string `json:"id" jsonschema:"Local record identifier"`
}

type proPage struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Page size, 1–50; defaults to 20"`
	Offset int `json:"offset,omitempty" jsonschema:"Offset from next_offset in the previous page"`
}

type proPipeline struct {
	proPage
	ID string `json:"id" jsonschema:"Local sales pipeline identifier"`
}

type proOpportunitySearch struct {
	proPage
	Query    string `json:"query,omitempty"`
	Pipeline string `json:"pipeline_id,omitempty"`
	Stage    string `json:"stage_id,omitempty"`
	Status   string `json:"status,omitempty"`
}

type proBusiness struct {
	proPage
	Filter string `json:"filter,omitempty" jsonschema:"Business filter, e.g. status:open currency:USD"`
}

type proSummary struct {
	proBusiness
	Period string `json:"period,omitempty" jsonschema:"Comparison period; defaults to month"`
}

type proForecast struct {
	proBusiness
	AsOf string `json:"as_of,omitempty"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type proReport struct {
	proPage
	ID string `json:"id" jsonschema:"Identifier or name of an existing report definition"`
}

type proRuleList struct {
	proPage
	Archived bool `json:"archived,omitempty"`
}

type proRecordRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type proSimulation struct {
	RuleID string        `json:"rule_id"`
	Record *proRecordRef `json:"record,omitempty"`
	After  string        `json:"after,omitempty"`
	Limit  int           `json:"limit,omitempty"`
}

type proRuns struct {
	RuleID string `json:"rule_id,omitempty"`
	Before int64  `json:"before,omitempty" jsonschema:"Sequence cursor from the previous page"`
	Limit  int    `json:"limit,omitempty"`
}

type proPlaybook struct {
	PlaybookID       string            `json:"playbook_id"`
	PlaybookRevision int               `json:"playbook_revision,omitempty"`
	Record           proRecordRef      `json:"record"`
	RecordRevision   int               `json:"record_revision,omitempty"`
	StartOn          string            `json:"start_on,omitempty"`
	Dates            map[string]string `json:"dates,omitempty"`
	IncludeOptional  bool              `json:"include_optional,omitempty"`
	InvocationID     string            `json:"invocation_id,omitempty" jsonschema:"Stable caller-generated key for retrying the same proposal"`
}

func (s *Server) addProfessional(ctx context.Context, manager *ext.Manager) {
	actions := manager.MCPActions(ctx)
	addProfessionalTool[proOpportunitySearch](s, manager, actions, "opportunities_search", "opportunities.read", "Search an authorized page of Professional opportunity summaries.", false)
	addProfessionalTool[proID](s, manager, actions, "opportunities_get", "opportunities.read", "Get opportunity facts without notes or custom fields.", false)
	addProfessionalTool[proPipeline](s, manager, actions, "sales_pipeline_get", "pipelines.read", "Get Professional sales pipeline and a page of stage facts.", false)
	addProfessionalTool[proSummary](s, manager, actions, "business_summary", "reports.read", "Summarize Professional business performance by currency.", false)
	addProfessionalTool[proBusiness](s, manager, actions, "business_briefing", "reports.read", "Read an authorized page of daily briefing items.", false)
	addProfessionalTool[proForecast](s, manager, actions, "business_forecast", "reports.read", "Read Professional forecast aggregates by currency.", false)
	addProfessionalTool[proBusiness](s, manager, actions, "business_attention", "reports.read", "Read an authorized page of Professional attention items.", false)
	addProfessionalTool[proReport](s, manager, actions, "reports_run", "reports.read", "Compute a saved Professional report; does not export or save anything.", false)
	addProfessionalTool[proRuleList](s, manager, actions, "automation_rules_list", "automation.inspect", "List a page of Professional automation rule summaries.", false)
	addProfessionalTool[proID](s, manager, actions, "automation_rule_inspect", "automation.inspect", "Inspect Professional rule metadata, trigger, and action kinds.", false)
	addProfessionalTool[proSimulation](s, manager, actions, "automation_simulate", "automation.simulate", "Simulate an existing rule without writing records or creating actions.", false)
	addProfessionalTool[proRuns](s, manager, actions, "automation_runs_list", "automation.inspect", "Read a page of Professional automation execution summaries.", false)
	addProfessionalTool[proPlaybook](s, manager, actions, "playbooks_preview", "playbooks.preview", "Preview a Professional playbook for one record without writing anything.", false)
	addProfessionalTool[proPlaybook](s, manager, actions, "playbooks_start", "playbooks.execute", "Save an approval-only Professional playbook proposal. Cannot approve or apply any action.", true)
}

func addProfessionalTool[I any](s *Server, manager *ext.Manager, actions map[string]bool, name, scope, description string, write bool) {
	if !actions[name] {
		return
	}
	addTool(s, name, scope, description, write, func(ctx context.Context, in I) (json.RawMessage, error) {
		data, err := json.Marshal(in)
		if err != nil || len(data) > 16<<10 {
			return nil, problem("invalid_input", "Professional tool input is too large or invalid")
		}
		result, err := manager.RunMCP(ctx, ext.MCPRequest{Action: name, Actor: s.profile.Name, Input: data})
		var e *ext.ProtocolError
		if errors.As(err, &e) {
			// Never send arbitrary extension diagnostics to the MCP client.
			switch e.Code {
			case ext.CodePermissionDenied, ext.CodeEntitlement:
				return nil, problem("permission_denied", "Professional operation not permitted")
			case ext.CodeInvalid:
				return nil, problem("invalid_input", "invalid Professional tool input")
			case ext.CodeNotFound:
				return nil, problem("not_found", "Professional record not found")
			case ext.CodeConflict:
				return nil, problem("conflict", "Professional record changed; refresh and retry")
			}
		}
		return result, err
	})
}
