package transfer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

// WriteArchive writes the versioned JSON form of a CRM export.
func WriteArchive(w io.Writer, a domain.Archive) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(a)
}

// ReadArchiveFile reads and checks an archive. Files over 50 MiB are refused.
func ReadArchiveFile(path string) (domain.Archive, error) {
	var a domain.Archive
	f, e := os.Open(path)
	if e != nil {
		return a, e
	}
	defer func() { _ = f.Close() }()
	data, e := io.ReadAll(io.LimitReader(f, 50<<20+1))
	if e != nil {
		return a, e
	}
	if len(data) > 50<<20 {
		return a, fmt.Errorf("file is larger than 50 MiB")
	}
	return ReadArchive(data)
}

func ReadArchive(data []byte) (domain.Archive, error) {
	var a domain.Archive
	if e := json.Unmarshal(data, &a); e != nil {
		return a, fmt.Errorf("not a TideContacts CRM export: %w", e)
	}
	if a.Format != domain.ArchiveFormat {
		return a, fmt.Errorf("not a TideContacts CRM export (format %q)", a.Format)
	}
	if a.Version != domain.ArchiveVersion {
		return a, fmt.Errorf("export version %d is not supported (this build reads version %d)", a.Version, domain.ArchiveVersion)
	}
	return a, nil
}

// Counts describes an archive for the confirmation shown before an import.
func Counts(a domain.Archive) string {
	items := 0
	for _, p := range a.Pipelines {
		items += len(p.Items)
	}
	return fmt.Sprintf("%d activities, %d tasks, %d reminders, %d projects, %d pipelines (%d items), %d follow-up schedules, %d dates, %d collections",
		len(a.Activities), len(a.Tasks), len(a.Reminders), len(a.Projects), len(a.Pipelines), items, len(a.Follow), len(a.Dates), len(a.Collection))
}

// Summary describes an import result in one line.
func Summary(r domain.ImportReport) string {
	order := []string{"activities", "tasks", "reminders", "projects", "pipelines", "items", "follow-ups", "dates", "relationship notes", "collections"}
	parts := []string{}
	for _, k := range order {
		if r.Added[k] > 0 || r.Skipped[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d new", k, r.Added[k])+map[bool]string{true: fmt.Sprintf(" (%d already present)", r.Skipped[k])}[r.Skipped[k] > 0])
		}
	}
	if len(parts) == 0 {
		return "nothing to import"
	}
	return strings.Join(parts, " · ")
}

func icsEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`).Replace(s)
}

// icsLine folds a content line at 75 octets (74 plus the leading space on continuation
// lines) without splitting a UTF-8 sequence.
func icsLine(w *bytes.Buffer, line string) {
	limit := 75
	for len(line) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(line[cut]) {
			cut--
		}
		w.WriteString(line[:cut] + "\r\n ")
		line, limit = line[cut:], 74
	}
	w.WriteString(line + "\r\n")
}

var icsDays = []string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

func icsRule(r domain.Recurrence, timed bool) string {
	if r.IsZero() || r.AfterCompletion {
		return ""
	}
	parts := []string{"FREQ=" + strings.ToUpper(r.Freq)}
	if r.Interval > 1 {
		parts = append(parts, fmt.Sprintf("INTERVAL=%d", r.Interval))
	}
	if len(r.ByDay) > 0 {
		days := []string{}
		for _, d := range r.ByDay {
			days = append(days, icsDays[d%7])
		}
		parts = append(parts, "BYDAY="+strings.Join(days, ","))
	}
	if r.MonthDay > 0 {
		parts = append(parts, fmt.Sprintf("BYMONTHDAY=%d", r.MonthDay))
	}
	if r.Until != "" {
		if t, ok := domain.ParseDay(r.Until); ok {
			if timed {
				parts = append(parts, "UNTIL="+t.Format("20060102")+"T235959Z")
			} else {
				parts = append(parts, "UNTIL="+t.Format("20060102"))
			}
		}
	}
	if r.Count > 0 {
		parts = append(parts, fmt.Sprintf("COUNT=%d", r.Count))
	}
	return strings.Join(parts, ";")
}

// WriteICalendar writes open and finished tasks as VTODO and pending reminders as
// VEVENT with a display alarm. Export only: iCalendar import is not supported.
// Differences from TideContacts: "monthly on 31" follows RFC 5545 (short months are
// skipped, not clamped), and "after completion" rules are not representable, so they
// are exported as one-off items with an X-TIDECONTACTS-REPEAT note.
func WriteICalendar(w io.Writer, a domain.Archive, now time.Time) error {
	var b bytes.Buffer
	icsLine(&b, "BEGIN:VCALENDAR")
	icsLine(&b, "VERSION:2.0")
	icsLine(&b, "PRODID:-//TideContacts//CRM export//EN")
	icsLine(&b, "CALSCALE:GREGORIAN")
	stamp := now.UTC().Format("20060102T150405Z")
	for _, t := range a.Tasks {
		icsLine(&b, "BEGIN:VTODO")
		icsLine(&b, "UID:"+t.ID+"@tidecontacts")
		icsLine(&b, "DTSTAMP:"+stamp)
		icsLine(&b, "SUMMARY:"+icsEscape(t.Title))
		if t.Description != "" {
			icsLine(&b, "DESCRIPTION:"+icsEscape(t.Description))
		}
		timed := false
		if d, ok := domain.ParseDay(t.Due); ok {
			if c, e := time.Parse("15:04", t.DueTime); e == nil {
				timed = true
				prop := "DUE"
				if t.Timezone != "" {
					prop += ";TZID=" + t.Timezone
				}
				icsLine(&b, prop+":"+d.Format("20060102")+"T"+c.Format("150405"))
			} else {
				icsLine(&b, "DUE;VALUE=DATE:"+d.Format("20060102"))
			}
		}
		if d, ok := domain.ParseDay(t.Start); ok {
			icsLine(&b, "DTSTART;VALUE=DATE:"+d.Format("20060102"))
		}
		switch t.Status {
		case "completed":
			icsLine(&b, "STATUS:COMPLETED")
			if c, e := time.Parse(time.RFC3339, t.CompletedAt); e == nil {
				icsLine(&b, "COMPLETED:"+c.UTC().Format("20060102T150405Z"))
			}
		case "canceled":
			icsLine(&b, "STATUS:CANCELLED")
		case "waiting":
			icsLine(&b, "STATUS:IN-PROCESS")
		default:
			icsLine(&b, "STATUS:NEEDS-ACTION")
		}
		if p := map[int]int{3: 1, 2: 5, 1: 9}[t.Priority]; p > 0 {
			icsLine(&b, fmt.Sprintf("PRIORITY:%d", p))
		}
		if len(t.Tags) > 0 {
			tags := make([]string, len(t.Tags))
			for i, tag := range t.Tags {
				tags[i] = icsEscape(tag)
			}
			icsLine(&b, "CATEGORIES:"+strings.Join(tags, ","))
		}
		if t.Open() {
			if rule := icsRule(t.Recurrence, timed); rule != "" {
				icsLine(&b, "RRULE:"+rule)
			} else if t.Recurrence.AfterCompletion {
				icsLine(&b, "X-TIDECONTACTS-REPEAT:"+icsEscape(t.Recurrence.String()))
			}
		}
		if who := domain.RefNames(t.Entities); who != "" {
			icsLine(&b, "X-TIDECONTACTS-WITH:"+icsEscape(who))
		}
		icsLine(&b, "END:VTODO")
	}
	for _, m := range a.Reminders {
		at := m.Due().UTC()
		icsLine(&b, "BEGIN:VEVENT")
		icsLine(&b, "UID:"+m.ID+"@tidecontacts")
		icsLine(&b, "DTSTAMP:"+stamp)
		icsLine(&b, "DTSTART:"+at.Format("20060102T150405Z"))
		icsLine(&b, "DURATION:PT0S")
		icsLine(&b, "SUMMARY:"+icsEscape(m.Title))
		if m.Note != "" {
			icsLine(&b, "DESCRIPTION:"+icsEscape(m.Note))
		}
		if rule := icsRule(m.Recurrence, true); rule != "" {
			icsLine(&b, "RRULE:"+rule)
		}
		icsLine(&b, "BEGIN:VALARM")
		icsLine(&b, "ACTION:DISPLAY")
		icsLine(&b, "DESCRIPTION:"+icsEscape(m.Title))
		icsLine(&b, "TRIGGER:PT0S")
		icsLine(&b, "END:VALARM")
		icsLine(&b, "END:VEVENT")
	}
	icsLine(&b, "END:VCALENDAR")
	_, e := w.Write(b.Bytes())
	return e
}
