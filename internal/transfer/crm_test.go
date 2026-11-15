package transfer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

func TestArchiveReadRejectsForeignAndFutureFiles(t *testing.T) {
	var buf bytes.Buffer
	a := domain.Archive{Format: domain.ArchiveFormat, Version: domain.ArchiveVersion, Tasks: []domain.Task{{ID: "t", Title: "x"}}}
	if e := WriteArchive(&buf, a); e != nil {
		t.Fatal(e)
	}
	got, e := ReadArchive(buf.Bytes())
	if e != nil || len(got.Tasks) != 1 {
		t.Fatal(got, e)
	}
	for _, bad := range []string{`{}`, `not json`, `{"format":"tidecontacts-crm","version":99}`, `{"format":"other","version":1}`} {
		if _, e := ReadArchive([]byte(bad)); e == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestPublishFileNeverClobbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if e := PublishFile(path, func(w io.Writer) error { _, e := w.Write([]byte("first")); return e }); e != nil {
		t.Fatal(e)
	}
	if e := PublishFile(path, func(w io.Writer) error { _, e := w.Write([]byte("second")); return e }); e == nil {
		t.Fatal("overwrote an existing file")
	}
	if b, _ := os.ReadFile(path); string(b) != "first" {
		t.Fatal(string(b))
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary file left behind", entries)
	}
}

func TestICalendarShape(t *testing.T) {
	rule, _ := domain.ParseRecurrence("every 2 weeks on mon wed until 2027-01-01")
	after, _ := domain.ParseRecurrence("weekly after completion")
	a := domain.Archive{
		Tasks: []domain.Task{
			{ID: "a", Title: "Send, then; call\nback", Description: strings.Repeat("long ", 40), Status: "scheduled", Priority: 3, Due: "2026-11-20", DueTime: "09:30", Timezone: "Europe/Paris", Recurrence: rule, Tags: []string{"x,y"}, Entities: []domain.Ref{{Name: "Alex"}}},
			{ID: "b", Title: "Done 界", Status: "completed", Due: "2026-11-01", CompletedAt: "2026-11-02T10:00:00Z", Recurrence: rule},
			{ID: "c", Title: "After", Status: "next", Recurrence: after, Due: "2026-11-05"},
		},
		Reminders: []domain.Reminder{{ID: "r", Title: "Ping", At: time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)}},
	}
	var buf bytes.Buffer
	if e := WriteICalendar(&buf, a, time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)); e != nil {
		t.Fatal(e)
	}
	out := buf.String()
	for _, want := range []string{"BEGIN:VCALENDAR\r\n", "UID:a@tidecontacts", "DUE;TZID=Europe/Paris:20261120T093000", "PRIORITY:1", `SUMMARY:Send\, then\; call\nback`,
		"RRULE:FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;UNTIL=20270101T235959Z", "STATUS:COMPLETED", "COMPLETED:20261102T100000Z", "X-TIDECONTACTS-REPEAT:weekly after completion",
		"CATEGORIES:x\\,y", "X-TIDECONTACTS-WITH:Alex", "BEGIN:VALARM", "TRIGGER:PT0S", "DTSTART:20261114T090000Z", "END:VCALENDAR\r\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// A completed occurrence does not repeat; only the open one carries the rule.
	if strings.Count(out, "RRULE:") != 1 {
		t.Error("RRULE count")
	}
	for _, line := range strings.Split(out, "\r\n") {
		if len(line) > 75 {
			t.Errorf("unfolded line of %d octets: %q", len(line), line)
		}
	}
	if strings.Contains(out, "\n\n") || strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Error("bare newline in output")
	}
}
