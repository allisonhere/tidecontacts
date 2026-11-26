package webapp

import (
	"net/http"
	"testing"
)

func TestProfessionalRoutesAreClosedWithoutAnApprovedExtension(t *testing.T) {
	_, h := webFixture(t, nil)
	profile := webCall(t, h, http.MethodGet, "profile", nil, 200, "")
	if got := string(profile); got != `{"demo":true,"professional":[]}` {
		t.Fatalf("profile = %s; Community-only must advertise no Professional operations", got)
	}
	for _, tc := range []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "pro/pulse", 403, "permission_denied"},
		{http.MethodGet, "pro/opportunities/abc", 403, "permission_denied"},
		{http.MethodGet, "pro/replay", 400, "invalid_input"},
		{http.MethodGet, "pro/replay?date=yesterday", 400, "invalid_input"},
		{http.MethodGet, "pro/opportunities?offset=-1", 400, "invalid_input"},
		{http.MethodGet, "pro/opportunities?offset=x", 400, "invalid_input"},
		{http.MethodGet, "pro", 404, "not_found"},
		{http.MethodPost, "pro/pulse", 404, "not_found"},
		{http.MethodDelete, "pro/rules/x", 404, "not_found"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			webCall(t, h, tc.method, tc.path, nil, tc.status, tc.code)
		})
	}
	// Community data stays available while Professional is absent.
	webCall(t, h, http.MethodGet, "people?limit=1", nil, 200, "")
}
