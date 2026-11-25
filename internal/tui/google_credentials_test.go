package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func googleSourceIndex(t *testing.T) int {
	t.Helper()
	for i, s := range importSources {
		if s.google {
			return i
		}
	}
	t.Fatal("no Google import source configured")
	return -1
}

// TestGoogleContactsCredentialsForm exercises the form up to the point of
// submitting valid credentials. It stops short of delivering the returned
// connect command, which would make a real network call to Google.
func TestGoogleContactsCredentialsForm(t *testing.T) {
	m, _ := browser(t)
	idx := googleSourceIndex(t)
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = idx
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "directory-books" {
		t.Fatal("expected directory-books stage for the Google source")
	}
	m, cmd = key(m, "n")
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "google-credentials" || m.contacts.transfer.googleFocus != 0 {
		t.Fatal("expected google-credentials stage focused on email")
	}
	m, _ = key(m, "alex@example.com")
	if m.contacts.transfer.googleEmail.Value() != "alex@example.com" {
		t.Fatal("email field did not capture input")
	}
	m, cmd = key(m, "tab")
	m = deliver(m, cmd)
	if m.contacts.transfer.googleFocus != 1 {
		t.Fatal("tab did not move focus to the app password field")
	}
	m, _ = key(m, "an-app-password")
	if m.contacts.transfer.googlePassword.Value() != "an-app-password" {
		t.Fatal("password field did not capture input")
	}
	m, cmd = press(m, tea.KeyEnter)
	if cmd == nil || !m.contacts.transfer.working {
		t.Fatal("expected enter with valid credentials to start connecting")
	}
	if !strings.Contains(m.contacts.transfer.message, "Connecting to Google") {
		t.Fatal(m.contacts.transfer.message)
	}
}

func TestGoogleContactsCredentialsValidation(t *testing.T) {
	m, _ := browser(t)
	idx := googleSourceIndex(t)
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = idx
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	m, cmd = key(m, "n")
	m = deliver(m, cmd)
	m, cmd = press(m, tea.KeyEnter)
	if cmd != nil || m.contacts.transfer.stage != "google-credentials" {
		t.Fatal("empty submission should not proceed")
	}
	if !strings.Contains(m.contacts.transfer.message, "email") {
		t.Fatal(m.contacts.transfer.message)
	}
	m, _ = key(m, "not-an-email")
	m, cmd = press(m, tea.KeyEnter)
	if cmd != nil || m.contacts.transfer.stage != "google-credentials" {
		t.Fatal("an email without @ should not proceed")
	}
}

// TestGoogleContactsFormAcceptsLetterB guards against the app password and
// email fields losing a literal "b" keystroke to the accounts-list back
// shortcut other list stages use — Google app passwords are lowercase, so a
// bare "b" is common in a real one.
func TestGoogleContactsFormAcceptsLetterB(t *testing.T) {
	m, _ := browser(t)
	idx := googleSourceIndex(t)
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = idx
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	m, cmd = key(m, "n")
	m = deliver(m, cmd)
	m, _ = key(m, "bob@example.com")
	if m.contacts.transfer.stage != "google-credentials" {
		t.Fatal("typing a letter b in the email field should not leave the form")
	}
	if m.contacts.transfer.googleEmail.Value() != "bob@example.com" {
		t.Fatalf("email field lost input: %q", m.contacts.transfer.googleEmail.Value())
	}
	m, cmd = key(m, "tab")
	m = deliver(m, cmd)
	m, _ = key(m, "abcd-efgh-ijkl-bnop")
	if m.contacts.transfer.stage != "google-credentials" {
		t.Fatal("typing a letter b in the password field should not leave the form")
	}
	if m.contacts.transfer.googlePassword.Value() != "abcd-efgh-ijkl-bnop" {
		t.Fatalf("password field lost input: %q", m.contacts.transfer.googlePassword.Value())
	}
}

func TestGoogleContactsBooksListFiltersBySource(t *testing.T) {
	m, _ := browser(t)
	idx := googleSourceIndex(t)
	m, _, _ = m.startTransfer("import-contacts")
	m.contacts.transfer.source = idx
	m, cmd := press(m, tea.KeyEnter)
	m = deliver(m, cmd)
	if m.contacts.transfer.stage != "directory-books" {
		t.Fatal("expected directory-books stage")
	}
	// No Google accounts are connected in this fresh store, and the local
	// vcarddir fixtures used elsewhere in this package must never leak into
	// the Google-filtered list.
	for _, book := range m.contacts.transfer.books {
		if book.Path != "" {
			t.Fatalf("unexpected book in Google-filtered list: %s", book.Path)
		}
	}
}
