package tui

import (
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tideui"
	"github.com/charmbracelet/lipgloss"
)

func contactOverview(r tideui.Renderer, c domain.Contact, width int) string {
	body := r.Styles.DetailBody.Width(width)
	meta := r.Styles.DetailMeta.Width(width)
	lines := []string{body.Bold(true).Render(domain.SafeText(c.DisplayName))}
	var status []string
	if c.Favorite {
		status = append(status, "* Favorite")
	}
	if c.Archived {
		status = append(status, "Archived")
	}
	if c.Deleted {
		status = append(status, "Deleted — restore through the command palette")
	}
	if len(status) > 0 {
		lines = append(lines, meta.Render(strings.Join(status, " · ")))
	}

	theme := r.Styles.Theme
	entryIndex := 0
	type overviewField struct {
		label, value string
		muted        bool
	}
	var fields []overviewField
	field := func(label, value string) {
		if text := domain.SafeText(value); strings.TrimSpace(text) != "" {
			fields = append(fields, overviewField{label: domain.SafeText(label), value: text})
		}
	}
	section := func(title string) {
		if len(fields) == 0 {
			return
		}
		lines = append(lines, "", r.RenderSectionDivider(tideui.SectionDivider{Label: title, Width: width}, theme.Bg))
		const labelWidth = 15
		for _, f := range fields {
			bg := contactBand(theme, entryIndex)
			entryIndex++
			base := body.Background(bg).Foreground(tideui.Legible(theme.Fg, bg, 4.5))
			labelStyle := base.Foreground(tideui.Legible(theme.BorderFocus, bg, 4.5))
			switch {
			case f.muted:
				lines = append(lines, meta.Background(bg).Foreground(tideui.Legible(theme.Dimmed, bg, 4.5)).Render(f.value))
			case f.label == "":
				lines = append(lines, base.Render(f.value))
			case width < 40 || lipgloss.Width(f.label) > labelWidth:
				lines = append(lines, labelStyle.Render(f.label), base.Render(f.value))
			default:
				value := base.Width(width - labelWidth - 2).Render(f.value)
				label := labelStyle.Width(labelWidth + 2).Height(lipgloss.Height(value)).Render(f.label)
				lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, label, value))
			}
		}
		fields = nil
	}
	for _, v := range c.Methods {
		label := v.Kind
		if v.Label != "" && v.Label != v.Kind {
			label += " · " + v.Label
		}
		if v.Preferred {
			label += " ★"
		}
		value := v.Value
		if v.Kind == "address" {
			a := v.Address
			value = overviewParts(", ", a.POBox, a.Extended, a.Street, a.City, a.Region, a.PostalCode, a.Country)
		}
		field(label, value)
	}
	field("Website", c.Website)
	section("Contact details")
	for _, v := range c.Memberships {
		status := "past"
		if v.Current {
			status = "current"
		}
		field(v.Organization, overviewParts(" · ", v.Title, v.Department, overviewParts(" – ", v.Start, v.End), status))
	}
	field("Industry", c.Industry)
	for _, member := range c.Members {
		field("Member", overviewParts(" · ", member.TargetName, member.Type, member.Notes))
	}
	section("Organizations")
	field("Nickname", c.Nickname)
	field("Pronunciation", c.Phonetic)
	field("Aliases", strings.Join(c.Aliases, ", "))
	field("Birthday", c.Birthday)
	field("Anniversary", c.Anniversary)
	field("Tags", strings.Join(c.Tags, ", "))
	section("About")
	field("", c.Biography)
	section("Biography")
	field("", clip(c.Notes, 400))
	section("Private notes")
	return strings.Join(lines, "\n")
}

func overviewParts(separator string, values ...string) string {
	var filled []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			filled = append(filled, value)
		}
	}
	return strings.Join(filled, separator)
}
