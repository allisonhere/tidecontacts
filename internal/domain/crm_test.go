package domain

import (
	"testing"
	"time"
)

func TestRecurrenceParseAndNext(t *testing.T) {
	cases := []struct{ rule, anchor, prev, want string }{
		{"daily", "2026-01-01", "2026-01-01", "2026-01-02"},
		{"every 3 days", "2026-01-01", "2026-01-05", "2026-01-07"},
		{"weekly", "2026-11-13", "2026-11-13", "2026-11-20"},
		{"every 2 weeks", "2026-11-13", "2026-11-13", "2026-11-27"},
		{"weekly on mon wed", "2026-11-09", "2026-11-09", "2026-11-11"},
		{"weekly on mon wed", "2026-11-09", "2026-11-11", "2026-11-16"},
		{"every 2 weeks on mon, wed", "2026-11-09", "2026-11-11", "2026-11-23"},
		{"weekdays", "2026-11-13", "2026-11-13", "2026-11-16"},
		{"monthly", "2026-01-31", "2026-01-31", "2026-02-28"},
		{"monthly", "2026-01-31", "2026-02-28", "2026-03-31"}, // the anchor day is remembered after a short month
		{"monthly on 15", "2026-01-31", "2026-01-31", "2026-02-15"},
		{"quarterly", "2026-01-10", "2026-01-10", "2026-04-10"},
		{"yearly", "2024-02-29", "2024-02-29", "2025-02-28"},
		{"yearly", "2024-02-29", "2027-02-28", "2028-02-29"},
		{"2w", "2026-11-13", "2026-11-13", "2026-11-27"},
		{"mon fri", "2026-11-13", "2026-11-13", "2026-11-16"},
	}
	for _, c := range cases {
		r, e := ParseRecurrence(c.rule)
		if e != nil {
			t.Fatalf("%s: %v", c.rule, e)
		}
		r.Anchor = c.anchor
		got, ok := r.Next(c.prev)
		if !ok || got != c.want {
			t.Errorf("%s after %s: got %s want %s", c.rule, c.prev, got, c.want)
		}
		again, e := ParseRecurrence(r.String())
		again.Anchor = c.anchor
		if next, _ := again.Next(c.prev); e != nil || next != c.want {
			t.Errorf("%s: String() %q does not round-trip (%v)", c.rule, r.String(), e)
		}
	}
	for _, bad := range []string{"sometimes", "every 0 days", "weekly on 31", "monthly on mon", "daily until soon", "x0"} {
		if _, e := ParseRecurrence(bad); e == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if r, e := ParseRecurrence("none"); e != nil || !r.IsZero() {
		t.Fatal("none must clear")
	}
}

func TestAdvanceIsBoundedAndHonoursLimits(t *testing.T) {
	r, _ := ParseRecurrence("daily")
	r.Anchor = "2020-01-01"
	// Six years overdue: one successor on or after today, not thousands.
	if next, ok := r.Advance("2020-01-01", "2026-11-13", "2026-11-13", 0); !ok || next != "2026-11-13" {
		t.Fatal(next, ok)
	}
	r, _ = ParseRecurrence("weekly x2")
	r.Anchor = "2026-11-13"
	if _, ok := r.Advance("2026-11-13", "2026-11-13", "2026-11-13", 0); !ok {
		t.Fatal("first successor refused")
	}
	if _, ok := r.Advance("2026-11-20", "2026-11-20", "2026-11-20", 1); ok {
		t.Fatal("count exceeded")
	}
	r, _ = ParseRecurrence("weekly until 2026-11-18")
	r.Anchor = "2026-11-13"
	if _, ok := r.Advance("2026-11-13", "2026-11-13", "2026-11-13", 0); ok {
		t.Fatal("until exceeded")
	}
	r, _ = ParseRecurrence("every 2 weeks after completion")
	r.Anchor = "2026-11-01"
	if next, ok := r.Advance("2026-11-01", "2026-11-10", "2026-11-10", 0); !ok || next != "2026-11-24" {
		t.Fatal(next, ok)
	}
}

func TestDaylightSavingAndZoneBoundaries(t *testing.T) {
	ny, e := time.LoadLocation("America/New_York")
	if e != nil {
		t.Skip("no tzdata")
	}
	// A 09:00 daily task keeps its wall-clock time across both 2026 transitions (Mar 8, Nov 1).
	r, _ := ParseRecurrence("daily")
	for _, day := range []string{"2026-03-07", "2026-10-31"} {
		r.Anchor = day
		next, _ := r.Next(day)
		before, _ := At(day, "09:00", ny)
		after, _ := At(next, "09:00", ny)
		if after.Hour() != 9 || before.Hour() != 9 {
			t.Fatal("wall clock drifted", before, after)
		}
		if gap := after.Sub(before); gap == 24*time.Hour {
			t.Fatalf("%s: expected a 23h or 25h day across the transition, got %v", day, gap)
		}
	}
	// A wall-clock time inside the spring-forward gap still resolves to a real instant that day.
	gap, ok := At("2026-03-08", "02:30", ny)
	if !ok || Day(gap) != "2026-03-08" {
		t.Fatal(gap)
	}
	// The same instant is different calendar days in different zones; "today" follows the zone.
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	instant := time.Date(2026, 11, 13, 23, 30, 0, 0, ny)
	if Day(instant) != "2026-11-13" || Day(instant.In(tokyo)) != "2026-11-14" {
		t.Fatal("zone boundary")
	}
	if Zone("Not/AZone", ny) != ny || Zone("", nil) != time.UTC {
		t.Fatal("zone fallback")
	}
}

func TestDateWordsAndAnnual(t *testing.T) {
	today := "2026-11-13" // a Friday
	for in, want := range map[string]string{"today": today, "tomorrow": "2026-11-14", "fri": "2026-11-20", "monday": "2026-11-16", "+3d": "2026-11-16",
		"+2w": "2026-11-27", "+1m": "2026-12-13", "2027-01-05": "2027-01-05", "01-05": "2027-01-05", "none": ""} {
		got, e := ParseWhen(in, today)
		if e != nil || got != want {
			t.Errorf("%s: %s %v", in, got, e)
		}
	}
	if _, e := ParseWhen("someday", today); e == nil {
		t.Fatal("accepted someday")
	}
	if NextAnnual("1990-11-13", today) != today || NextAnnual("--11-12", today) != "2027-11-12" || NextAnnual("2000-02-29", "2027-01-01") != "2027-02-28" {
		t.Fatal("annual")
	}
	if AddInterval("2026-01-31", 1, "month") != "2026-02-28" || Relative("2026-11-10", today) != "3d overdue" {
		t.Fatal("interval")
	}
}

func TestFollowUpReasons(t *testing.T) {
	today := "2026-11-13"
	// No cadence and no date: never due, whatever the history looks like.
	f := FollowUp{}
	f.Evaluate(today)
	if f.Due != "" {
		t.Fatal("inferred a follow-up without a rule")
	}
	i, never, e := ParseInterval("every two weeks")
	if e != nil || never || i != (Interval{2, "week"}) {
		t.Fatal(i, e)
	}
	f = FollowUp{Interval: i, SetAt: "2026-11-01", LastInteraction: "2026-10-20", LastType: "call"}
	f.Evaluate(today)
	if f.Due != "2026-11-03" || f.Reason == "" {
		t.Fatal(f.Due, f.Reason)
	}
	f = FollowUp{Interval: Interval{1, "month"}, SetAt: "2026-11-01"}
	f.Evaluate(today)
	if f.Due != "2026-12-01" {
		t.Fatal(f.Due)
	}
	f = FollowUp{Interval: i, Never: true}
	f.Evaluate(today)
	if f.Due != "" {
		t.Fatal("never ignored")
	}
	f = FollowUp{NextDate: "2026-11-10", SnoozedUntil: "2026-11-20"}
	f.Evaluate(today)
	if f.Due != "2026-11-20" {
		t.Fatal(f.Due)
	}
	if _, never, _ := ParseInterval("never"); !never {
		t.Fatal("never")
	}
	if q, _, _ := ParseInterval("quarterly"); q != (Interval{3, "month"}) {
		t.Fatal(q)
	}
	if _, _, e := ParseInterval("weekly on mon"); e == nil {
		t.Fatal("accepted weekday cadence")
	}
}
