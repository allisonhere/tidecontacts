package ext

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestGraphicalReadsRequireApprovedProviderGrantAndEntitlement(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	entitled := testMCPEntitlements{"pro.opportunities": true, "pro.graphical-read": true}
	m := f.manager(Options{Entitlements: entitled})
	const manifest = `permissions = ["cli.register", "storage.sql"]
[storage]
sql = true
[[cli]]
resource = "opportunities"
actions = ["list", "get"]
[[provides]]
id = "pro.opportunities"
version = 1
[[provides]]
id = "pro.graphical-read"
version = 1
`
	read := func(operation, id string) error {
		_, err := m.RunGraphicalRead(ctx, GraphicalQuery{Operation: operation, ID: id})
		return err
	}
	wantCode := func(err error, code string) {
		t.Helper()
		var p *ProtocolError
		if !errors.As(err, &p) || p.Code != code {
			t.Fatalf("expected %s, got %v", code, err)
		}
	}
	if got := m.GraphicalActions(ctx); len(got) != 0 {
		t.Fatalf("operations advertised without an extension: %v", got)
	}
	wantCode(read("opportunities", ""), CodePermissionDenied)

	// The script only accepts the dedicated method with the exact derived argv.
	dir := f.add(graphicalProvider, manifest, `read line
case "$line" in
  *'"method":"graphical.read"'*'"resource":"opportunities"'*'"argv":["list","--limit=50","--offset=0"]'*) ;;
  *'"method":"graphical.read"'*'"argv":["get","opp-1","--history"]'*) ;;
  *) exit 73 ;;
esac
id=$(printf '%s' "$line" | sed 's/.*"request_id":"\([^"]*\)".*/\1/')
printf '{"api":1,"type":"response","request_id":"%s","ok":true,"data":{"data":[{"id":"x"}],"count":1}}\n' "$id"
`)
	approve := func(granted ...string) {
		t.Helper()
		sum, err := fileSHA256(filepath.Join(dir, "run.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if err := m.update(graphicalProvider, func(e *stateEntry) { e.Enabled, e.ExecSHA256, e.Granted = true, sum, granted }); err != nil {
			t.Fatal(err)
		}
	}
	wantCode(read("opportunities", ""), CodePermissionDenied) // installed, not approved
	approve("storage.sql")
	wantCode(read("opportunities", ""), CodePermissionDenied) // CLI grant missing
	approve("cli.register", "storage.sql")

	if got := m.GraphicalActions(ctx); !slices.Equal(got, []string{"opportunities"}) {
		t.Fatalf("operations = %v; only provided, entitled, declared reads may be offered", got)
	}
	res, err := m.RunGraphicalRead(ctx, GraphicalQuery{Operation: "opportunities"})
	if err != nil || res.Count != 1 {
		t.Fatalf("approved read failed: %+v, %v", res, err)
	}
	if _, err := m.RunGraphicalRead(ctx, GraphicalQuery{Operation: "opportunities", ID: "opp-1"}); err != nil {
		t.Fatalf("detail read failed: %v", err)
	}
	for _, bad := range []GraphicalQuery{
		{Operation: "pulse"},                          // not provided or declared
		{Operation: "automation"},                     // not a route name
		{Operation: "opportunities", ID: "-all"},      // option injection
		{Operation: "opportunities", Currency: "usd"}, // not an ISO code shape
		{Operation: "opportunities", Offset: -1},      // negative page
		{Operation: "customers", ID: "id"},            // list-only operation
		{Operation: "opportunities", Query: string(make([]byte, 600))},
	} {
		if err := read2(m, bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}

	delete(entitled, "pro.opportunities")
	wantCode(read("opportunities", ""), CodePermissionDenied) // entitlement withdrawn
	entitled["pro.opportunities"] = true
	approve("storage.sql")
	wantCode(read("opportunities", ""), CodePermissionDenied) // grant revoked
}

func read2(m *Manager, q GraphicalQuery) error {
	_, err := m.RunGraphicalRead(context.Background(), q)
	return err
}
