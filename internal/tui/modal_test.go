package tui

import (
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
			m = do(m, "y", "5", "n", "Task due:today", "enter", "e")
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
