package tui

import (
	"fmt"
	"strings"
	"time"

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

	labelStyle := r.Styles.DetailBody.Foreground(tideui.Legible(r.Styles.Theme.BorderFocus, r.Styles.Theme.Bg, 4.5))
	var fields []string
	field := func(label, value string) {
		value = domain.SafeText(value)
		if strings.TrimSpace(value) == "" {
			return
		}
		label = domain.SafeText(label)
		const labelWidth = 15
		switch {
		case label == "":
			fields = append(fields, body.Render(value))
		case width < 40 || lipgloss.Width(label) > labelWidth:
			fields = append(fields, labelStyle.Width(width).Render(label), body.Render(value))
		default:
			fields = append(fields, lipgloss.JoinHorizontal(lipgloss.Top,
				labelStyle.Width(labelWidth).Render(label), "  ",
				body.Width(width-labelWidth-2).Render(value)))
		}
	}
	section := func(title string) {
		if len(fields) == 0 {
			return
		}
		lines = append(lines, "", r.RenderSectionDivider(tideui.SectionDivider{Label: title, Width: width}, r.Styles.Theme.Bg))
		lines = append(lines, fields...)
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
	field("Source", c.Source)
	updated := c.UpdatedAt
	if at, err := time.Parse(time.RFC3339, updated); err == nil {
		updated = at.UTC().Format("Jan 2, 2006 · 15:04 UTC")
	}
	field("Updated", updated)
	field("UUID", c.ID)
	fields = append(fields, meta.Render(fmt.Sprintf("%d relationships · %d custom fields", len(c.Relationships)+len(c.Incoming), len(c.Fields))))
	section("Record details")
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
