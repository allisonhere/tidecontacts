package tui

import (
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tideui"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestModalsStayInBoundsInColour(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)
	// Styled cells are where width bugs hide; the other bounds tests run without colour.
	for _, theme := range []string{"dark", "light"} {
		for _, size := range [][2]int{{100, 30}, {40, 12}, {1, 1}} {
			m, s := crmApp(t, 120, 34)
			m.cfg.Theme = theme
			seedPeople(t, s)
			m = deliver(m, m.refreshContacts())
			n, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = n.(Model)
			m = do(m, "n", "Jo Park")
			for sec := 0; sec < 7; sec++ {
				checkBounds(t, m.View(), size[0], size[1])
				m, _ = press(m, tea.KeyCtrlN)
				checkBounds(t, m.View(), size[0], size[1])
				m, _ = press(m, tea.KeyF6)
			}
			m, _ = press(m, tea.KeyEsc)
			checkBounds(t, m.View(), size[0], size[1])
			m = do(m, "y", "5", "n", "Task")
			if m.crm.form == nil {
				t.Fatal("form did not open", m.crm.err)
			}
			for i := range m.crm.form.fields {
				m.crm.form.selected = i
				checkBounds(t, m.View(), size[0], size[1])
			}
			m.crm.form = nil
			for _, k := range []string{"L", "D", "n"} {
				m = do(m, k)
				checkBounds(t, m.View(), size[0], size[1])
				m.crm.picker, m.crm.confirm, m.crm.prompt = nil, nil, nil
			}
		}
	}
}

func TestRecordFormsShareVerticalHeight(t *testing.T) {
	for _, size := range [][2]int{{120, 32}, {80, 24}, {40, 12}} {
		m, _ := crmApp(t, size[0], size[1])
		r := tideui.NewRenderer(m.theme(), tideui.StyleOptions{Density: tideui.Compact})
		u := newSoftUI(r)
		expected := 0
		check := func(name string, panel *tideui.Overlay) {
			t.Helper()
			height := lipgloss.Height(panel.Content)
			if expected == 0 {
				expected = height
			}
			if height != expected {
				t.Fatalf("%s at %v: height %d, want %d", name, size, height, expected)
			}
		}
		for _, kind := range []string{"person", "organization"} {
			editor := newEditor(domain.NewContact(kind))
			m.contacts.editor = &editor
			for section := range editorSections {
				editor.section = section
				editor.selected = 0
				editor.rebuild()
				check(kind+editorSections[section], m.editorPanel(u))
			}
		}
		m.contacts.editor = nil
		for _, open := range []func() tea.Cmd{
			func() tea.Cmd { return m.taskForm(domain.Task{}) },
			func() tea.Cmd { return m.activityForm(domain.Activity{}) },
			func() tea.Cmd { return m.projectForm(domain.Project{}) },
			func() tea.Cmd { return m.pipelineForm(domain.Pipeline{}) },
		} {
			_ = open()
			for i := range m.crm.form.fields {
				m.crm.form.selected = i
				check(m.crm.form.title, m.formPanel(u))
			}
		}
	}
}
