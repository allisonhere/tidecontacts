package googlecontacts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Google's real CardDAV member hrefs are opaque IDs with no ".vcf" suffix,
// unlike most CardDAV servers — this fixture models that quirk rather than
// the more common filename-suffixed style, so the resourcetype-based
// filtering (not a filename check) is what's actually under test.
const testPropfindResponse = `<?xml version="1.0" encoding="utf-8" ?>
<multistatus xmlns="DAV:">
  <response><href>/lists/default/</href><propstat><prop><resourcetype><collection/></resourcetype></prop></propstat></response>
  <response><href>/lists/default/3c1f9a2b</href><propstat><prop><resourcetype/></prop></propstat></response>
  <response><href>/lists/default/7d8e0c41</href><propstat><prop><resourcetype/></prop></propstat></response>
</multistatus>`

const testAlexCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:alex-1\r\nFN:Alex Example\r\nEMAIL:alex@example.com\r\nEND:VCARD\r\n"
const testSamCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:sam-1\r\nFN:Sam Example\r\nEMAIL:sam@example.com\r\nEND:VCARD\r\n"

func testServer(t *testing.T, wantUser, wantPass string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/lists/default/", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != wantUser || p != wantPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Method != "PROPFIND" {
			t.Fatalf("expected PROPFIND, got %s", r.Method)
		}
		if r.Header.Get("Depth") != "1" {
			t.Fatalf("expected Depth: 1, got %q", r.Header.Get("Depth"))
		}
		w.WriteHeader(207)
		_, _ = w.Write([]byte(testPropfindResponse))
	})
	mux.HandleFunc("/lists/default/3c1f9a2b", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != wantUser || p != wantPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(testAlexCard))
	})
	mux.HandleFunc("/lists/default/7d8e0c41", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != wantUser || p != wantPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(testSamCard))
	})
	return httptest.NewServer(mux)
}

func TestFetchFromListsAndDecodesCards(t *testing.T) {
	srv := testServer(t, "alex@example.com", "an-app-password")
	defer srv.Close()

	snapshot, err := fetchFrom(context.Background(), srv.URL+"/lists/default/", "alex@example.com", "an-app-password")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Path != "google://alex@example.com" {
		t.Fatalf("unexpected path: %s", snapshot.Path)
	}
	if len(snapshot.Cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(snapshot.Cards))
	}
	names := map[string]bool{}
	for _, c := range snapshot.Cards {
		names[c.Contact.DisplayName] = true
	}
	if !names["Alex Example"] || !names["Sam Example"] {
		t.Fatalf("missing expected contacts: %+v", names)
	}
}

func TestFetchFromRejectsBadPassword(t *testing.T) {
	srv := testServer(t, "alex@example.com", "an-app-password")
	defer srv.Close()

	_, err := fetchFrom(context.Background(), srv.URL+"/lists/default/", "alex@example.com", "wrong-password")
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected a rejected-credentials error, got %v", err)
	}
}

func TestIsAccountAndEmail(t *testing.T) {
	path := Path("alex@example.com")
	if !IsAccount(path) {
		t.Fatal("expected a google:// path to be recognized as an account")
	}
	if Email(path) != "alex@example.com" {
		t.Fatalf("unexpected email: %s", Email(path))
	}
	if IsAccount("/home/alex/contacts") {
		t.Fatal("local folder path misidentified as a Google account")
	}
}
