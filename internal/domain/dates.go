package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Calendar dates are stored as YYYY-MM-DD strings in the user's local calendar.
// Instants are stored as UTC RFC 3339 with the IANA zone they were recorded in.
const DateLayout = "2006-01-02"

// Day formats the calendar date of t in t's own location.
func Day(t time.Time) string { return t.Format(DateLayout) }

func ParseDay(s string) (time.Time, bool) {
	t, e := time.Parse(DateLayout, s)
	return t, e == nil
}

func AddDays(day string, n int) string {
	t, ok := ParseDay(day)
	if !ok {
		return day
	}
	return Day(t.AddDate(0, 0, n))
}

// DaysBetween returns b-a in whole calendar days.
func DaysBetween(a, b string) int {
	ta, ok1 := ParseDay(a)
	tb, ok2 := ParseDay(b)
	if !ok1 || !ok2 {
		return 0
	}
	return int(tb.Sub(ta).Hours() / 24)
}

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// clampDate builds a date, clamping the day to the month's length (31 -> 30, Feb 29 -> Feb 28).
func clampDate(year int, month time.Month, day int) time.Time {
	for month > 12 {
		month -= 12
		year++
	}
	for month < 1 {
		month += 12
		year--
	}
	return time.Date(year, month, min(day, daysIn(year, month)), 0, 0, 0, 0, time.UTC)
}

// AddInterval adds n days, weeks, months or years to a calendar date, clamping month ends.
func AddInterval(day string, n int, unit string) string {
	t, ok := ParseDay(day)
	if !ok {
		return day
	}
	switch unit {
	case "week":
		return Day(t.AddDate(0, 0, 7*n))
	case "month":
		return Day(clampDate(t.Year(), t.Month()+time.Month(n), t.Day()))
	case "year":
		return Day(clampDate(t.Year()+n, t.Month(), t.Day()))
	}
	return Day(t.AddDate(0, 0, n))
}

var weekdays = map[string]time.Weekday{"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday}

func weekday(s string) (time.Weekday, bool) {
	if len(s) < 3 {
		return 0, false
	}
	d, ok := weekdays[s[:3]]
	if ok && !strings.HasPrefix(d.String(), strings.ToUpper(s[:1])+s[1:]) {
		return 0, false
	}
	return d, ok
}

// ParseWhen resolves a date word relative to today. An empty result clears the date.
// Accepted: today, tomorrow, yesterday, weekday names (next occurrence), +3d/+2w/+1m/+1y,
// "next week", YYYY-MM-DD, MM-DD, and none/clear.
func ParseWhen(s, today string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	base, ok := ParseDay(today)
	if !ok {
		return "", fmt.Errorf("invalid reference date")
	}
	switch s {
	case "", "none", "clear", "never":
		return "", nil
	case "today", "tod", "now":
		return today, nil
	case "tomorrow", "tom":
		return AddDays(today, 1), nil
	case "yesterday":
		return AddDays(today, -1), nil
	case "week", "next week":
		return AddDays(today, 7), nil
	case "month", "next month":
		return AddInterval(today, 1, "month"), nil
	}
	if d, ok := weekday(s); ok {
		delta := (int(d) - int(base.Weekday()) + 7) % 7
		if delta == 0 {
			delta = 7
		}
		return AddDays(today, delta), nil
	}
	if strings.HasPrefix(s, "+") && len(s) > 2 {
		n, e := strconv.Atoi(s[1 : len(s)-1])
		units := map[byte]string{'d': "day", 'w': "week", 'm': "month", 'y': "year"}
		if unit, ok := units[s[len(s)-1]]; ok && e == nil && n >= 0 && n < 10000 {
			return AddInterval(today, n, unit), nil
		}
	}
	if t, ok := ParseDay(s); ok {
		return Day(t), nil
	}
	if t, e := time.Parse("01-02", s); e == nil {
		d := clampDate(base.Year(), t.Month(), t.Day())
		if d.Before(base) {
			d = clampDate(base.Year()+1, t.Month(), t.Day())
		}
		return Day(d), nil
	}
	return "", fmt.Errorf("unrecognized date %q; use today, tomorrow, fri, +3d, +2w or YYYY-MM-DD", s)
}

// ClockValid accepts an empty value or a 24-hour HH:MM time.
func ClockValid(s string) bool {
	if s == "" {
		return true
	}
	_, e := time.Parse("15:04", s)
	return e == nil && len(s) == 5
}

// At returns the instant of a calendar date and optional HH:MM wall-clock time in loc.
// A wall-clock time skipped by a daylight-saving gap resolves to an adjacent valid instant.
func At(day, clock string, loc *time.Location) (time.Time, bool) {
	d, ok := ParseDay(day)
	if !ok {
		return time.Time{}, false
	}
	h, m := 0, 0
	if clock != "" {
		c, e := time.Parse("15:04", clock)
		if e != nil {
			return time.Time{}, false
		}
		h, m = c.Hour(), c.Minute()
	}
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc), true
}

// Zone resolves an IANA zone name; an empty or unknown name falls back to fallback.
func Zone(name string, fallback *time.Location) *time.Location {
	if name != "" {
		if loc, e := time.LoadLocation(name); e == nil {
			return loc
		}
	}
	if fallback == nil {
		return time.UTC
	}
	return fallback
}

// NextAnnual returns the next occurrence on or after today of YYYY-MM-DD or --MM-DD.
// February 29 is observed on February 28 in common years.
func NextAnnual(date, today string) string {
	base, ok := ParseDay(today)
	if !ok {
		return ""
	}
	md := strings.TrimPrefix(date, "--")
	if len(date) == 10 && !strings.HasPrefix(date, "--") {
		md = date[5:]
	}
	t, e := time.Parse("01-02", md)
	if e != nil {
		// 02-29 does not parse in year 0 handling of some layouts; handle explicitly.
		if md != "02-29" {
			return ""
		}
		t = time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC)
	}
	d := clampDate(base.Year(), t.Month(), t.Day())
	if d.Before(base) {
		d = clampDate(base.Year()+1, t.Month(), t.Day())
	}
	return Day(d)
}

// Relative describes a date relative to today in a few words.
func Relative(day, today string) string {
	if day == "" {
		return ""
	}
	n := DaysBetween(today, day)
	switch {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n > 1 && n < 7:
		if t, ok := ParseDay(day); ok {
			return t.Weekday().String()[:3] + " " + day[5:]
		}
	case n < 0:
		return fmt.Sprintf("%dd overdue", -n)
	}
	return day
}
