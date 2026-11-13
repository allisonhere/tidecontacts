package tui

import (
	"github.com/allisonhere/ripple"
	"github.com/allisonhere/tideui"
	bkey "github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// multiline is the one adapter between TideContacts and the Ripple editor. Every
// multi-line field (contact notes, biography, activity bodies, task descriptions,
// project and pipeline notes) goes through it, so key bindings, clipboard wiring and
// rendering stay identical everywhere and Ripple can be replaced in one place.
type multiline struct{ model ripple.Model }

func newMultiline(value string) multiline {
	m := ripple.New()
	m.SetClipboard(systemClipboard{})
	keys := ripple.DefaultKeyMap()
	// The host form owns Tab and Esc, so Ripple's own find bar stays off.
	keys.Find.SetEnabled(false)
	keys.Copy = bkey.NewBinding(bkey.WithKeys("alt+c"))
	m.SetKeyMap(keys)
	m.SetValue(value)
	return multiline{m}
}

func (e *multiline) Focus()           { e.model.Focus() }
func (e *multiline) Blur()            { e.model.Blur() }
func (e multiline) Value() string     { return e.model.Value() }
func (e *multiline) SetSize(w, h int) { e.model.SetSize(max(1, w), max(1, h)) }
func (e multiline) Update(msg tea.Msg) (multiline, tea.Cmd) {
	var cmd tea.Cmd
	e.model, cmd = e.model.Update(msg)
	return e, cmd
}

func (e multiline) Render(r tideui.Renderer, w, h int) string {
	e.SetSize(w, h)
	return e.model.View(ripple.Options{Cursor: r.Styles.ItemSelected.Render(" "), Selected: func(s string) string { return r.Styles.ItemSelected.Render(s) }})
}
