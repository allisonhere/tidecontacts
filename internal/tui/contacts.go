package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tideui"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"time"
)

var profileViews = []string{"Overview", "Contact methods", "Organizations", "Notes", "Relationships", "Custom fields", "History", "Dossier", "Timeline"}

type contactsState struct {
	marked                   map[string]bool
	cancelWrite              context.CancelFunc
	exitAfter                bool
	repo                     app.Contacts
	ctx                      context.Context
	rows                     []domain.Summary
	selected, view, scroll   int
	contact                  domain.Contact
	history                  []domain.History
	listSeq, detailSeq       uint64
	cancelList, cancelDetail context.CancelFunc
	loading, busy            bool
	err, order, confirm      string
	editor                   *contactEditor
	transfer                 *transferUI
	undo                     *domain.Contact
}
type listResult struct {
	seq  uint64
	kind string
	rows []domain.Summary
	err  error
}
type detailResult struct {
	seq     uint64
	contact domain.Contact
	history []domain.History
	err     error
}
type saveResult struct {
	contacts []domain.Contact
	previous *domain.Contact
	err      error
	action   string
}
type searchTick struct{ seq uint64 }

func (m Model) isContacts() bool {
	return m.contacts.repo != nil && (m.state.Active == 0 || m.state.Active == 1)
}
func (m Model) contactKind() string {
	if m.state.Active == 1 {
		return "organization"
	}
	return "person"
}
func (m *Model) refreshContacts() tea.Cmd {
	c := &m.contacts
	if !m.isContacts() {
		return nil
	}
	if c.cancelList != nil {
		c.cancelList()
	}
	c.listSeq++
	seq := c.listSeq
	kind := m.contactKind()
	query, err := search.Parse(m.queries[m.state.Active])
	if err != nil {
		c.err = err.Error()
		c.loading = false
		return nil
	}
	c.err = ""
	c.loading = true
	ctx, cancel := context.WithCancel(c.ctx)
	c.cancelList = cancel
	repo, order := c.repo, c.order
	return func() tea.Msg { rows, e := repo.List(ctx, kind, query, order); return listResult{seq, kind, rows, e} }
}
func (m *Model) loadContact() tea.Cmd {
	c := &m.contacts
	if c.cancelDetail != nil {
		c.cancelDetail()
	}
	c.detailSeq++
	c.contact = domain.Contact{}
	c.history = nil
	c.scroll = 0
	if len(c.rows) == 0 {
		return nil
	}
	c.selected = min(max(0, c.selected), len(c.rows)-1)
	id := c.rows[c.selected].ID
	seq := c.detailSeq
	repo := c.repo
	ctx, cancel := context.WithCancel(c.ctx)
	c.cancelDetail = cancel
	return func() tea.Msg {
		contact, e := repo.Contact(ctx, id)
		var history []domain.History
		if e == nil {
			history, e = repo.History(ctx, id)
		}
		return detailResult{seq, contact, history, e}
	}
}
func (m Model) updateContacts(msg tea.Msg) (Model, tea.Cmd, bool) {
	c := &m.contacts
	if k, ok := msg.(tea.KeyMsg); ok && c.busy && k.String() == "ctrl+c" {
		c.exitAfter = true
		if c.cancelWrite != nil {
			c.cancelWrite()
		}
		return m, nil, true
	}
	switch v := msg.(type) {
	case searchTick:
		if v.seq != c.listSeq {
			return m, nil, true
		}
		return m, m.refreshContacts(), true
	case listResult:
		if v.seq != c.listSeq || v.kind != m.contactKind() {
			return m, nil, true
		}
		c.loading = false
		if v.err != nil {
			if v.err != context.Canceled {
				c.err = v.err.Error()
			}
			return m, nil, true
		}
		old := c.contact.ID
		c.rows = v.rows
		c.selected = 0
		for i, row := range c.rows {
			if row.ID == old {
				c.selected = i
			}
		}
		return m, m.loadContact(), true
	case detailResult:
		if v.seq != c.detailSeq {
			return m, nil, true
		}
		if v.err != nil {
			if v.err != context.Canceled {
				c.err = v.err.Error()
			}
			return m, nil, true
		}
		c.contact = v.contact
		c.history = v.history
		return m, nil, true
	case saveResult:
		c.busy = false
		if c.cancelWrite != nil {
			c.cancelWrite()
			c.cancelWrite = nil
		}
		if c.exitAfter {
			return m, tea.Quit, true
		}
		if v.err != nil {
			c.err = v.err.Error()
			if c.editor != nil {
				c.editor.err = v.err.Error()
			}
			return m, nil, true
		}
		c.err = ""
		c.editor = nil
		c.undo = v.previous
		m.feedback = v.action + " saved"
		if len(v.contacts) > 0 {
			c.contact = v.contacts[0]
		}
		if c.transfer != nil {
			c.transfer.stage = "result"
			c.transfer.message = fmt.Sprintf("Imported %d records atomically. Existing contacts were not overwritten.", len(v.contacts))
			c.undo = nil
		}
		return m, m.refreshContacts(), true
	}
	if c.transfer != nil {
		return m.updateTransfer(msg)
	}
	if c.editor != nil {
		return m.updateEditor(msg)
	}
	if c.confirm != "" {
		if k, ok := msg.(tea.KeyMsg); ok {
			switch k.String() {
			case "esc", "n":
				c.confirm = ""
			case "y":
				if c.confirm == "delete" {
					old := c.contact
					next := old
					next.Deleted = true
					c.confirm = ""
					return m, m.saveContact(next, &old, "delete"), true
				}
			}
		}
		return m, nil, true
	}
	return m, nil, false
}
func (m *Model) saveContact(next domain.Contact, previous *domain.Contact, action string) tea.Cmd {
	c := &m.contacts
	if c.busy {
		return nil
	}
	c.busy = true
	c.err = ""
	ctx, cancel := context.WithCancel(c.ctx)
	c.cancelWrite = cancel
	repo := c.repo
	return func() tea.Msg {
		saved, e := repo.SaveContacts(ctx, []domain.Contact{next}, action)
		return saveResult{saved, previous, e, action}
	}
}
func (m Model) contactAction(id string) (Model, tea.Cmd, bool) {
	if !m.isContacts() {
		return m, nil, false
	}
	c := &m.contacts
	if c.busy {
		return m, nil, true
	}
	switch id {
	case "down", "up":
		delta := 1
		if id == "up" {
			delta = -1
		}
		if m.focus == 0 {
			next := min(max(0, c.selected+delta), max(0, len(c.rows)-1))
			if next != c.selected {
				c.selected = next
				return m, m.loadContact(), true
			}
		} else {
			c.scroll = max(0, c.scroll+delta)
		}
		return m, nil, true
	case "page-down", "page-up":
		delta := max(1, m.height-8)
		if id == "page-up" {
			delta = -delta
		}
		if m.focus == 0 {
			c.selected = min(max(0, c.selected+delta), max(0, len(c.rows)-1))
			return m, m.loadContact(), true
		}
		c.scroll = max(0, c.scroll+delta)
		return m, nil, true
	case "top", "bottom":
		if m.focus == 0 {
			c.selected = 0
			if id == "bottom" {
				c.selected = max(0, len(c.rows)-1)
			}
			return m, m.loadContact(), true
		}
		c.scroll = 0
		if id == "bottom" {
			c.scroll = 1000000
		}
		return m, nil, true
	case "activate":
		m.focus = 1
		return m, nil, true
	case "search":
		m.focus = 0
		m.editing = true
		return m, m.search.Focus(), true
	case "mark-contact":
		if len(c.rows) > 0 {
			if c.marked == nil {
				c.marked = map[string]bool{}
			}
			id := c.rows[c.selected].ID
			c.marked[id] = !c.marked[id]
		}
		return m, nil, true
	case "new-contact":
		ed := newEditor(domain.NewContact(m.contactKind()))
		c.editor = &ed
		return m, ed.focusCmd(), true
	case "edit-contact":
		if c.contact.ID == "" {
			c.err = "Select a record first"
			return m, nil, true
		}
		ed := newEditor(c.contact)
		c.editor = &ed
		return m, ed.focusCmd(), true
	case "favorite", "archive", "restore", "delete-contact":
		if c.contact.ID == "" {
			c.err = "Select a record first"
			return m, nil, true
		}
		old := c.contact
		next := old
		switch id {
		case "favorite":
			next.Favorite = !next.Favorite
		case "archive":
			next.Archived = !next.Archived
		case "restore":
			next.Deleted = false
			next.Archived = false
		case "delete-contact":
			c.confirm = "delete"
			return m, nil, true
		}
		return m, m.saveContact(next, &old, id), true
	case "undo-contact":
		if c.undo == nil {
			c.err = "No recoverable action in this session"
			return m, nil, true
		}
		prior := *c.undo
		repo, ctx := c.repo, c.ctx
		c.busy = true
		return m, func() tea.Msg {
			latest, e := repo.Contact(ctx, prior.ID)
			if e == nil {
				if latest.Revision != prior.Revision+1 {
					e = fmt.Errorf("record changed after this action; refusing to undo newer changes")
				} else {
					prior.Revision = latest.Revision
				}
			}
			if e != nil {
				return saveResult{err: e}
			}
			saved, e := repo.SaveContacts(ctx, []domain.Contact{prior}, "undo")
			return saveResult{contacts: saved, err: e, action: "undo"}
		}, true
	case "profile-next", "profile-previous":
		d := 1
		if id == "profile-previous" {
			d = -1
		}
		c.view = (c.view + d + len(profileViews)) % len(profileViews)
		c.scroll = 0
		m.focus = 1
		return m, nil, true
	case "sort":
		orders := []string{"name", "organization", "updated", "added"}
		i := 0
		for j, v := range orders {
			if v == c.order {
				i = j
			}
		}
		c.order = orders[(i+1)%len(orders)]
		m.feedback = "Sort: " + c.order
		return m, m.refreshContacts(), true
	case "density":
		if m.cfg.Density == "comfortable" {
			m.cfg.Density = "compact"
		} else {
			m.cfg.Density = "comfortable"
		}
		return m, nil, true
	case "import-contacts", "export-all", "export-selected", "export-filtered":
		return m.startTransfer(id)
	}
	return m, nil, false
}
func (m Model) contactPanes(r tideui.Renderer) [3]tideui.Pane {
	c := m.contacts
	navWidth := max(1, int(float64(m.width)*m.left.Value())-4)
	detailWidth := max(1, int(float64(m.width)*(1-m.left.Value()-m.right.Value()))-4)
	if m.mode() == tideui.SidebarOnly {
		detailWidth = max(1, m.width-int(float64(m.width)*m.left.Value())-4)
	}
	if m.mode() == tideui.Tabbed {
		navWidth = max(1, m.width-4)
		detailWidth = navWidth
	}
	rows := []string{}
	input := m.search
	input.Width = max(1, navWidth-3)
	input.TextStyle = r.Styles.DetailBody
	if m.editing {
		rows = append(rows, input.View())
	} else {
		rows = append(rows, clip("/ "+m.queries[m.state.Active], navWidth))
	}
	rowHeight := 1
	if m.cfg.Density == "comfortable" {
		rowHeight = 2
	}
	visible := max(1, (m.height-7)/rowHeight)
	start := max(0, c.selected-visible+1)
	end := min(len(c.rows), start+visible)
	for i := start; i < end; i++ {
		row := c.rows[i]
		prefix := "  "
		if i == c.selected {
			prefix = "> "
		}
		if c.marked[row.ID] {
			prefix += "[x] "
		}
		if row.Favorite {
			prefix += "* "
		}
		suffix := row.Organization
		if suffix == "" {
			suffix = row.Tags
		}
		text := domain.SafeText(row.Name)
		if suffix != "" && rowHeight == 1 {
			text += " · " + domain.SafeText(suffix)
		}
		rows = append(rows, r.RenderRow(tideui.Row{Prefix: prefix, Text: text, Selected: i == c.selected && m.focus == 0}, navWidth))
		if rowHeight == 2 {
			rows = append(rows, r.Styles.DetailMeta.Render(clip("  "+domain.SafeText(suffix), navWidth)))
		}
	}
	if c.loading {
		rows = append(rows, "Searching...")
	} else if len(c.rows) == 0 {
		rows = append(rows, "No matching contacts.", "n create  I import")
	}
	if c.err != "" {
		rows = append(rows, r.Styles.DetailBody.Foreground(r.Styles.Theme.Error).Render("! "+clip(domain.SafeText(c.err), navWidth)))
	}
	title := fmt.Sprintf("%s %d/%d", domain.Workspaces()[m.state.Active].Name, min(c.selected+1, len(c.rows)), len(c.rows))
	content := m.profileContent()
	content = r.Styles.DetailBody.Width(detailWidth).Render(domain.SafeText(content))
	lines := strings.Split(content, "\n")
	offset := min(c.scroll, max(0, len(lines)-max(1, m.height-5)))
	if c.view >= 7 && c.contact.ID != "" {
		// Dossier and Timeline are link lists: the cursor follows records instead of scrolling text.
		rows := append([]crmRow{txt(c.contact.DisplayName)}, m.crm.dossier...)
		if m.crm.dossierFor != c.contact.ID {
			rows = append(rows[:1], dim("Loading…"))
		}
		rendered, at := m.renderRows(r, rows, m.crm.dossierCursor+1, m.focus == 1, detailWidth)
		content, offset = strings.Join(rendered, "\n"), max(0, at-max(1, m.height-6)+2)
	}
	contextLines := []string{"Profile views"}
	for i, name := range profileViews {
		prefix := "  "
		if i == c.view {
			prefix = "> "
		}
		contextLines = append(contextLines, prefix+name)
	}
	contextLines = append(contextLines, "", "[ / ] change view", "n new  e edit", "f favorite  a archive", "D delete  u undo", "I import  E export", "T task  A log activity", "F follow-up  d date  ! flag", "", "Sort: "+c.order)
	if n := len(m.crm.nav); n > 0 {
		contextLines = append(contextLines, "", clip("Esc ← "+domain.SafeText(m.crm.nav[n-1].name), max(1, int(float64(m.width)*m.right.Value())-4)))
	}
	if c.confirm == "delete" {
		content = "Delete " + domain.SafeText(c.contact.DisplayName) + "?\n\nThis creates a recoverable tombstone.\nRelationships and history are retained.\n\ny delete   n / Esc cancel"
		offset = 0
	}
	if c.busy {
		contextLines = append(contextLines, "", "Saving transaction...")
	}
	return [3]tideui.Pane{{Title: title, Content: strings.Join(rows, "\n"), Focused: m.focus == 0}, {Title: profileViews[c.view], Content: content, ScrollOffset: offset, Focused: m.focus == 1}, {Title: "Context", Content: strings.Join(contextLines, "\n"), Focused: m.focus == 2}}
}
func (m Model) profileContent() string {
	c := m.contacts.contact
	if c.ID == "" {
		if len(m.contacts.rows) > 0 {
			return "Loading contact..."
		}
		return "Your address book is ready.\n\nPress n to create a contact or I to import a CSV/vCard file.\n\nSearch examples:\n  alex\n  @github.com\n  tag:developer\n  org:\"Riverside Studio\"\n  is:favorite\n  has:phone\n\n1 People   2 Organizations"
	}
	lines := []string{c.DisplayName}
	if c.Favorite {
		lines = append(lines, "* Favorite")
	}
	if c.Archived {
		lines = append(lines, "Archived")
	}
	if c.Deleted {
		lines = append(lines, "Deleted — restore through the command palette")
	}
	lines = append(lines, "")
	methods := func() {
		for _, v := range c.Methods {
			value := v.Value
			if v.Kind == "address" {
				value = v.Address.String()
			}
			pref := ""
			if v.Preferred {
				pref = " * preferred"
			}
			lines = append(lines, v.Kind+" / "+v.Label+pref, value)
			if m.contacts.view == 1 {
				lines = append(lines, "  "+v.Verification+" · source: "+v.Source, "  "+v.Notes)
			}
			lines = append(lines, "")
		}
	}
	organizations := func() {
		for _, v := range c.Memberships {
			status := "past"
			if v.Current {
				status = "current"
			}
			lines = append(lines, v.Organization+" — "+v.Title, v.Department+" · "+v.Start+" – "+v.End+" · "+status, "")
		}
		if c.Kind == "organization" {
			lines = append(lines, "Website: "+c.Website, "Industry: "+c.Industry)
			for _, member := range c.Members {
				lines = append(lines, member.TargetName+" — "+member.Type, member.Notes)
			}
		}
	}
	switch m.contacts.view {
	case 0:
		methods()
		organizations()
		if len(c.Tags) > 0 {
			lines = append(lines, "Tags: "+strings.Join(c.Tags, ", "))
		}
		if c.Birthday != "" {
			lines = append(lines, "Birthday: "+c.Birthday)
		}
		if c.Anniversary != "" {
			lines = append(lines, "Anniversary: "+c.Anniversary)
		}
		if c.Nickname != "" {
			lines = append(lines, "Nickname: "+c.Nickname)
		}
		if c.Phonetic != "" {
			lines = append(lines, "Pronunciation: "+c.Phonetic)
		}
		if len(c.Aliases) > 0 {
			lines = append(lines, "Aliases: "+strings.Join(c.Aliases, ", "))
		}
		lines = append(lines, "", c.Biography, "", clip(c.Notes, 400), "", fmt.Sprintf("%d relationships · %d custom fields", len(c.Relationships)+len(c.Incoming), len(c.Fields)), "Source: "+c.Source, "Updated: "+c.UpdatedAt, "UUID: "+c.ID)
	case 1:
		methods()
		if len(c.Methods) == 0 {
			lines = append(lines, "No contact methods. Press e to add them.")
		}
	case 2:
		organizations()
		if len(c.Memberships) == 0 && c.Kind == "person" {
			lines = append(lines, "No organization memberships. Press e to assign one.")
		}
	case 3:
		lines = append(lines, "Biography", c.Biography, "", "Private notes", c.Notes, "", "e edits both fields with multiline support.")
	case 4:
		for _, v := range c.Relationships {
			lines = append(lines, v.Type+" → "+v.TargetName, v.Notes, "")
		}
		for _, v := range c.Incoming {
			lines = append(lines, v.Type+" ← "+v.TargetName, v.Notes, "")
		}
		if len(c.Relationships)+len(c.Incoming) == 0 {
			lines = append(lines, "No outgoing relationships. Press e to add one.")
		}
	case 5:
		for _, v := range c.Fields {
			lines = append(lines, v.Name+" ["+v.Type+"]: "+v.Value)
		}
		if len(c.Fields) == 0 {
			lines = append(lines, "No custom fields. Press e to define one.")
		}
	case 6:
		for _, h := range m.contacts.history {
			lines = append(lines, fmt.Sprintf("r%d  %s", h.Revision, h.Action), h.At, "")
		}
		lines = append(lines, "Local record changes only. No email/activity integration.")
	}
	if m.contacts.view == 0 {
		compact := []string{}
		for _, line := range lines {
			if line == "" && len(compact) > 0 && compact[len(compact)-1] == "" {
				continue
			}
			compact = append(compact, line)
		}
		lines = compact
	}
	return strings.Join(lines, "\n")
}
func (m *Model) debounceSearch() tea.Cmd {
	c := &m.contacts
	if c.cancelList != nil {
		c.cancelList()
	}
	c.listSeq++
	seq := c.listSeq
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return searchTick{seq} })
}
func cloneContact(c domain.Contact) domain.Contact {
	b, _ := json.Marshal(c)
	var copy domain.Contact
	_ = json.Unmarshal(b, &copy)
	return copy
}
