package tui

import (
	"fmt"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/connector/googlecontacts"
	"github.com/allisonhere/tidecontacts/internal/connector/vcarddir"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
)

// directoryDisplay renders a connected book's identity for the books list
// and preview — the raw path for a local folder, or just the email for a
// Google account, since "google://" is an implementation detail.
func directoryDisplay(path string) string {
	if googlecontacts.IsAccount(path) {
		return "Google Contacts: " + googlecontacts.Email(path)
	}
	return path
}

type directoryBooksResult struct {
	owner *transferUI
	books []domain.ContactDirectory
	err   error
}
type directoryPreparedResult struct {
	owner    *transferUI
	snapshot domain.DirectorySnapshot
	revision int
	err      error
}
type directoryRefreshResult struct {
	owner   *transferUI
	reports []domain.DirectoryReport
	warning string
	err     error
	note    string
}

func (m Model) loadDirectoryBooks() tea.Cmd {
	t := m.contacts.transfer
	repo, ok := m.contacts.repo.(app.Directories)
	if !ok {
		t.message = "Directory connections are unavailable"
		return nil
	}
	t.working = true
	return func() tea.Msg {
		books, err := repo.ContactDirectories(m.contacts.ctx)
		return directoryBooksResult{t, books, err}
	}
}
func (m Model) prepareDirectory(path string) tea.Cmd {
	t := m.contacts.transfer
	if m.cfg.Demo {
		t.message = "Live address book connections are disabled in demo mode"
		return nil
	}
	repo, ok := m.contacts.repo.(app.Directories)
	if !ok {
		t.message = "Directory connections are unavailable"
		return nil
	}
	t.working = true
	if googlecontacts.IsAccount(path) {
		t.message = "Connecting to Google and validating contacts…"
	} else {
		t.message = "Reading and validating the whole folder…"
	}
	return func() tea.Msg {
		snapshot, revision, err := vcarddir.Prepare(m.contacts.ctx, path, repo)
		return directoryPreparedResult{t, snapshot, revision, err}
	}
}

// connectGoogle validates a Google account over CardDAV with an app
// password and, on success, saves the password to the keychain. Unlike
// prepareDirectory (which reconnects an already-known book), there is no
// book yet: the path is derived from the email the user just entered.
func (m Model) connectGoogle(email, password string) tea.Cmd {
	t := m.contacts.transfer
	if m.cfg.Demo {
		t.message = "Live address book connections are disabled in demo mode"
		return nil
	}
	repo, ok := m.contacts.repo.(app.Directories)
	if !ok {
		t.message = "Directory connections are unavailable"
		return nil
	}
	t.working = true
	t.message = "Connecting to Google and validating contacts…"
	ctx := m.contacts.ctx
	return func() tea.Msg {
		snapshot, revision, err := vcarddir.ConnectGoogle(ctx, email, password, repo)
		return directoryPreparedResult{t, snapshot, revision, err}
	}
}
func (m Model) refreshDirectories(owner *transferUI) tea.Cmd {
	repo, ok := m.contacts.repo.(app.Directories)
	if !ok || m.cfg.Demo {
		return nil
	}
	if owner != nil {
		owner.working = true
		owner.message = "Refreshing connected address books…"
	}
	return func() tea.Msg {
		reports, warning := vcarddir.RefreshAll(m.contacts.ctx, repo)
		return directoryRefreshResult{owner: owner, reports: reports, warning: warning}
	}
}
func (m Model) updateDirectories(msg tea.Msg) (Model, tea.Cmd, bool) {
	switch v := msg.(type) {
	case directoryBooksResult:
		if m.contacts.transfer != v.owner {
			return m, nil, true
		}
		t := v.owner
		t.working = false
		if v.err != nil {
			t.message = v.err.Error()
			return m, nil, true
		}
		books := []domain.ContactDirectory{}
		for _, book := range v.books {
			if googlecontacts.IsAccount(book.Path) == importSources[t.source].google {
				books = append(books, book)
			}
		}
		t.books = books
		t.selected = min(t.selected, max(0, len(t.books)-1))
		return m, nil, true
	case directoryPreparedResult:
		if m.contacts.transfer != v.owner {
			return m, nil, true
		}
		t := v.owner
		t.working = false
		if v.err != nil {
			t.message = v.err.Error()
			return m, nil, true
		}
		t.directorySnapshot = v.snapshot
		t.directoryRevision = v.revision
		t.stage = "directory-preview"
		unchanged := "Folder files remain unchanged."
		if googlecontacts.IsAccount(v.snapshot.Path) {
			unchanged = "Nothing is written back to Google."
		}
		t.message = fmt.Sprintf("%d cards validated. No contacts written. %s", len(v.snapshot.Cards), unchanged)
		return m, nil, true
	case directoryRefreshResult:
		note := v.note
		if note == "" {
			note = vcarddir.Summary(v.reports)
		}
		if v.err != nil {
			note = v.err.Error()
		}
		if v.warning != "" {
			note += " · " + v.warning
		}
		if v.owner != nil {
			if m.contacts.transfer == v.owner {
				v.owner.working = false
				v.owner.stage = "result"
				v.owner.message = note
			}
		} else if len(v.reports) > 0 || v.warning != "" {
			m.feedback = note
		}
		return m, tea.Batch(m.refreshContacts(), m.loadCRM()), true
	}
	return m, nil, false
}
func (m Model) updateDirectoryStage(key string) (Model, tea.Cmd, bool) {
	t := m.contacts.transfer
	repo, ok := m.contacts.repo.(app.Directories)
	if !ok {
		return m, nil, false
	}
	switch t.stage {
	case "directory-books":
		switch key {
		case "up", "k":
			t.selected = max(0, t.selected-1)
		case "down", "j":
			t.selected = min(max(0, len(t.books)-1), t.selected+1)
		case "n":
			if importSources[t.source].google {
				t.stage = "google-credentials"
				t.googleEmail.SetValue("")
				t.googlePassword.SetValue("")
				t.googleFocus = 0
				t.message = ""
				return m, t.focusGoogleField(), true
			}
			t.stage = "browse"
			return m, t.readDirectory("."), true
		case "p":
			if importSources[t.source].google {
				return m, nil, true
			}
			t.stage = "path"
			t.path.SetValue("")
			return m, t.path.Focus(), true
		case "r":
			return m, m.refreshDirectories(t), true
		case "b":
			t.stage = "source"
		case "enter":
			if len(t.books) > 0 {
				return m, m.prepareDirectory(t.books[t.selected].Path), true
			}
		case "x":
			if len(t.books) > 0 {
				t.stage = "directory-disconnect"
				if googlecontacts.IsAccount(t.books[t.selected].Path) {
					t.message = "Disconnect hides this account's cached contacts. The saved app password is removed from the system keychain."
				} else {
					t.message = "Disconnect hides this folder’s cached contacts. Local contacts and original files are untouched."
				}
			}
		}
		return m, nil, true
	case "directory-preview":
		if key == "enter" {
			t.stage = "directory-confirm"
			source := "this folder"
			if googlecontacts.IsAccount(t.directorySnapshot.Path) {
				source = "this Google account"
			}
			t.message = "Connect or refresh " + source + "? Cards become read-only. Missing cards are removed from the active address book on each successful refresh."
		}
		if key == "b" {
			t.stage = "directory-books"
			return m, m.loadDirectoryBooks(), true
		}
		return m, nil, true
	case "directory-confirm":
		if key == "n" {
			t.stage = "directory-preview"
		}
		if key == "y" {
			t.working = true
			t.message = "Applying folder snapshot atomically…"
			snapshot, revision, ctx := t.directorySnapshot, t.directoryRevision, m.contacts.ctx
			return m, func() tea.Msg {
				report, err := repo.ApplyContactDirectory(ctx, snapshot, revision)
				return directoryRefreshResult{owner: t, reports: []domain.DirectoryReport{report}, err: err}
			}, true
		}
		return m, nil, true
	case "directory-disconnect":
		if key == "n" {
			t.stage = "directory-books"
		}
		if key == "y" && len(t.books) > 0 {
			book, ctx := t.books[t.selected], m.contacts.ctx
			t.working = true
			note := "Folder disconnected. Cached contacts hidden; original files unchanged."
			if googlecontacts.IsAccount(book.Path) {
				note = "Google account disconnected. Cached contacts hidden; the saved app password is kept for reconnect."
			}
			return m, func() tea.Msg {
				err := repo.DisconnectContactDirectory(ctx, book.ID, book.Revision)
				return directoryRefreshResult{owner: t, err: err, note: note}
			}, true
		}
		return m, nil, true
	}
	return m, nil, false
}
func (m Model) directoryPanel(r tideui.Renderer) *tideui.Overlay {
	t := m.contacts.transfer
	u := newSoftUI(r)
	width, height := m.formSize(86)
	lines := []string{}
	google := importSources[t.source].google
	switch t.stage {
	case "directory-books":
		title, empty := "Connected vCard folders · startup and manual refresh", "No folders connected. Press n to browse or p to enter a path."
		hints := []string{"n", "add folder", "r", "refresh all", "p", "path", "x", "disconnect", "esc", "close"}
		if google {
			title, empty = "Connected Google accounts · startup and manual refresh", "No Google accounts connected. Press n to connect one."
			hints = []string{"n", "connect account", "r", "refresh all", "x", "disconnect", "esc", "close"}
		}
		lines = append(lines, u.note(title, width, false), u.blank(width))
		if len(t.books) == 0 {
			lines = append(lines, u.note(empty, width, false))
		}
		start, end := tideui.VisibleRange(len(t.books), t.selected, max(1, (height-8)/2))
		for i := start; i < end; i++ {
			book := t.books[i]
			status := fmt.Sprintf("%d records · last refreshed %s", book.Contacts, book.LastRefreshed)
			if book.LastRefreshed == "" {
				status = "disconnected · Enter to reconnect"
			}
			lines = append(lines, u.line(r.SoftRail(i == t.selected, u.base)+u.style(u.base, u.text).Render(domain.SafeText(directoryDisplay(book.Path))), width), u.note(status, width, false))
		}
		lines = append(lines, u.hints(width, hints...))
	case "directory-preview":
		label := "Folder"
		if googlecontacts.IsAccount(t.directorySnapshot.Path) {
			label = "The Google account"
		}
		lines = append(lines, u.note(domain.SafeText(directoryDisplay(t.directorySnapshot.Path)), width, false), u.blank(width), u.note(label+" stays the source of truth. These contacts cannot be edited here.", width, false), u.note("Refresh updates changed cards and hides removed cards. Local contacts stay separate.", width, false), u.hints(width, "enter", "review connection", "b", "folders", "esc", "cancel"))
	case "directory-confirm", "directory-disconnect":
		lines = append(lines, u.hints(width, "y", "confirm", "n", "back", "esc", "cancel"))
	}
	for _, line := range strings.Split(t.message, "\n") {
		lines = append(lines, u.note(domain.SafeText(line), width, false))
	}
	return u.panel("address books / "+strings.TrimPrefix(t.stage, "directory-"), width, lines)
}

// googleCredentialsPanel renders the email + app password form used to
// connect a new Google account.
func (m Model) googleCredentialsPanel(r tideui.Renderer) *tideui.Overlay {
	t := m.contacts.transfer
	u := newSoftUI(r)
	width, _ := m.formSize(70)
	labelW := min(14, max(8, width*2/5))
	cw := u.controlWidth(width, labelW)
	lines := []string{
		u.note("Connect a Google account with an app password", width, false),
		u.blank(width),
		u.row("email", t.googleFocus == 0, u.input(t.googleEmail, cw, t.googleFocus == 0), width, labelW),
		u.row("app password", t.googleFocus == 1, u.input(t.googlePassword, cw, t.googleFocus == 1), width, labelW),
		u.blank(width),
		u.note("Create one at myaccount.google.com/apppasswords — requires 2-Step Verification.", width, false),
		u.note("Contacts become read-only here. Nothing is written back to Google.", width, false),
	}
	for _, line := range strings.Split(t.message, "\n") {
		lines = append(lines, u.note(domain.SafeText(line), width, false))
	}
	lines = append(lines, u.hints(width, "tab", "field", "enter", "connect", "esc", "cancel"))
	return u.panel("google contacts / connect", width, lines)
}
