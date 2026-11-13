package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Recurrence is a small calendar-based rule. Occurrences are calendar dates, so a
// task due at 09:00 stays at 09:00 local time across daylight-saving transitions.
//
// Schedule mode walks a fixed grid from Anchor. AfterCompletion mode measures the
// interval from the completion date instead. Exactly one successor is ever created.
type Recurrence struct {
	Freq            string `json:"freq,omitempty"` // daily, weekly, monthly, yearly
	Interval        int    `json:"interval,omitempty"`
	ByDay           []int  `json:"by_day,omitempty"`    // time.Weekday values, weekly only
	MonthDay        int    `json:"month_day,omitempty"` // monthly only; clamped to month length
	Anchor          string `json:"anchor,omitempty"`    // first occurrence
	Until           string `json:"until,omitempty"`
	Count           int    `json:"count,omitempty"` // total occurrences; 0 is unlimited
	AfterCompletion bool   `json:"after_completion,omitempty"`
}

func (r Recurrence) IsZero() bool { return r.Freq == "" }

func (r Recurrence) every() int { return max(1, r.Interval) }

var freqUnits = map[string]string{"daily": "day", "weekly": "week", "monthly": "month", "yearly": "year"}

// ParseRecurrence reads rules such as "daily", "weekdays", "every 2 weeks",
// "weekly on mon,wed", "monthly on 31", "yearly", "every 3 months until 2027-01-01",
// "weekly x10" and "every 2 weeks after completion". Empty or "none" clears the rule.
func ParseRecurrence(s string) (Recurrence, error) {
	var r Recurrence
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(s, ",", " ")))
	if len(words) == 0 || words[0] == "none" || words[0] == "never" {
		return r, nil
	}
	fail := func() (Recurrence, error) {
		return Recurrence{}, fmt.Errorf("unrecognized recurrence %q; try daily, weekdays, every 2 weeks, weekly on mon wed, monthly on 31, yearly, until YYYY-MM-DD, x10, after completion", s)
	}
	unit := func(w string) string {
		w = strings.TrimSuffix(w, "s")
		switch w {
		case "day", "daily", "d":
			return "daily"
		case "week", "weekly", "w":
			return "weekly"
		case "month", "monthly", "m":
			return "monthly"
		case "year", "yearly", "annually", "annual", "y":
			return "yearly"
		case "quarter", "quarterly":
			return "quarterly"
		}
		return ""
	}
	i := 0
	if words[i] == "every" {
		i++
	}
	if i < len(words) {
		if n, e := strconv.Atoi(words[i]); e == nil {
			if n < 1 || n > 1000 {
				return fail()
			}
			r.Interval = n
			i++
		}
	}
	if i >= len(words) {
		return fail()
	}
	switch w := words[i]; {
	case w == "weekdays" || w == "weekday":
		r.Freq, r.ByDay = "weekly", []int{1, 2, 3, 4, 5}
	case unit(w) == "quarterly":
		r.Freq, r.Interval = "monthly", 3*max(1, r.Interval)
	case unit(w) != "":
		r.Freq = unit(w)
	default:
		// Compact forms such as 2w or 10d.
		if n, e := strconv.Atoi(w[:len(w)-1]); e == nil && n > 0 && n <= 1000 && unit(w[len(w)-1:]) != "" {
			r.Freq, r.Interval = unit(w[len(w)-1:]), n
		} else if _, ok := weekday(w); ok {
			r.Freq = "weekly"
			i-- // weekday list without "on"
			words = append(words[:i+1], append([]string{"on"}, words[i+1:]...)...)
		} else {
			return fail()
		}
	}
	for i++; i < len(words); i++ {
		switch w := words[i]; {
		case w == "on":
			for i+1 < len(words) {
				next := words[i+1]
				if d, ok := weekday(next); ok && r.Freq == "weekly" {
					r.ByDay = append(r.ByDay, int(d))
				} else if n, e := strconv.Atoi(strings.TrimRight(next, "stndrh")); e == nil && r.Freq == "monthly" && n >= 1 && n <= 31 {
					r.MonthDay = n
				} else {
					break
				}
				i++
			}
			if len(r.ByDay) == 0 && r.MonthDay == 0 {
				return fail()
			}
		case w == "until" && i+1 < len(words):
			i++
			if _, ok := ParseDay(words[i]); !ok {
				return fail()
			}
			r.Until = words[i]
		case w == "after" && i+1 < len(words) && strings.HasPrefix(words[i+1], "complet"):
			i++
			r.AfterCompletion = true
		case strings.HasPrefix(w, "x"):
			n, e := strconv.Atoi(w[1:])
			if e != nil || n < 1 {
				return fail()
			}
			r.Count = n
		default:
			return fail()
		}
	}
	if r.Interval == 1 {
		r.Interval = 0
	}
	sort.Ints(r.ByDay)
	return r, nil
}

func (r Recurrence) String() string {
	if r.IsZero() {
		return ""
	}
	out := r.Freq
	if r.every() > 1 {
		out = fmt.Sprintf("every %d %ss", r.every(), freqUnits[r.Freq])
	}
	if len(r.ByDay) > 0 {
		days := []string{}
		for _, d := range r.ByDay {
			days = append(days, strings.ToLower(time.Weekday(d).String()[:3]))
		}
		out += " on " + strings.Join(days, " ")
	}
	if r.MonthDay > 0 {
		out += fmt.Sprintf(" on %d", r.MonthDay)
	}
	if r.Until != "" {
		out += " until " + r.Until
	}
	if r.Count > 0 {
		out += fmt.Sprintf(" x%d", r.Count)
	}
	if r.AfterCompletion {
		out += " after completion"
	}
	return out
}

func mondayOf(t time.Time) time.Time {
	return t.AddDate(0, 0, -((int(t.Weekday()) + 6) % 7))
}

// Next returns the first scheduled date strictly after prev on the rule's grid.
func (r Recurrence) Next(prev string) (string, bool) {
	p, ok := ParseDay(prev)
	if !ok || r.IsZero() {
		return "", false
	}
	a, ok := ParseDay(r.Anchor)
	if !ok {
		a = p
	}
	n := r.every()
	switch r.Freq {
	case "daily":
		d := int(p.Sub(a).Hours() / 24)
		k := 0
		if d >= 0 {
			k = d/n + 1
		}
		return Day(a.AddDate(0, 0, k*n)), true
	case "weekly":
		days := map[int]bool{}
		for _, d := range r.ByDay {
			days[d] = true
		}
		if len(days) == 0 {
			days[int(a.Weekday())] = true
		}
		start := mondayOf(a)
		for d := maxTime(p, a.AddDate(0, 0, -1)).AddDate(0, 0, 1); d.Before(p.AddDate(0, 0, 7*n+8)) || d.Before(a.AddDate(0, 0, 8)); d = d.AddDate(0, 0, 1) {
			weeks := int(mondayOf(d).Sub(start).Hours()/24) / 7
			if weeks >= 0 && weeks%n == 0 && days[int(d.Weekday())] {
				return Day(d), true
			}
		}
	case "monthly", "yearly":
		day := a.Day()
		if r.Freq == "monthly" && r.MonthDay > 0 {
			day = r.MonthDay
		}
		step := n
		if r.Freq == "yearly" {
			step = 12 * n
		}
		months := (p.Year()-a.Year())*12 + int(p.Month()) - int(a.Month())
		for k := max(0, months/step-1); k < max(0, months/step)+3; k++ {
			c := clampDate(a.Year(), a.Month()+time.Month(k*step), day)
			if c.After(p) {
				return Day(c), true
			}
		}
	}
	return "", false
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// Advance computes the single successor of occurrence number occurrence (zero-based)
// that was due on prevDue and completed on completedOn. Missed occurrences before
// today are skipped rather than generated, so the work is always bounded.
func (r Recurrence) Advance(prevDue, completedOn, today string, occurrence int) (string, bool) {
	if r.IsZero() || r.Count > 0 && occurrence+1 >= r.Count {
		return "", false
	}
	var next string
	var ok bool
	if r.AfterCompletion || prevDue == "" {
		base := r
		base.Anchor, base.ByDay = completedOn, nil
		if r.Freq == "monthly" {
			base.MonthDay = 0
		}
		next, ok = base.Next(completedOn)
	} else {
		next, ok = r.Next(prevDue)
		for i := 0; ok && next < today && i < 5000; i++ {
			next, ok = r.Next(next)
		}
	}
	if !ok || r.Until != "" && next > r.Until {
		return "", false
	}
	return next, true
}

// Interval is a contact-specific follow-up cadence.
type Interval struct {
	Every int
	Unit  string // day, week, month
}

func (i Interval) IsZero() bool { return i.Every <= 0 }

func (i Interval) String() string {
	if i.IsZero() {
		return ""
	}
	switch {
	case i.Every == 1 && i.Unit == "week":
		return "weekly"
	case i.Every == 2 && i.Unit == "week":
		return "every two weeks"
	case i.Every == 1 && i.Unit == "month":
		return "monthly"
	case i.Every == 3 && i.Unit == "month":
		return "quarterly"
	case i.Every == 12 && i.Unit == "month":
		return "yearly"
	}
	return fmt.Sprintf("every %d %ss", i.Every, i.Unit)
}

// ParseInterval reads "2w", "every two weeks", "monthly", "quarterly", "45 days" and similar.
// The second result is true for "never".
func ParseInterval(s string) (Interval, bool, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "never" || s == "off" {
		return Interval{}, true, nil
	}
	for word, n := range map[string]string{"two": "2", "three": "3", "four": "4", "six": "6"} {
		s = strings.ReplaceAll(s, word, n)
	}
	r, e := ParseRecurrence(s)
	if e != nil || r.IsZero() || len(r.ByDay) > 0 || r.MonthDay > 0 || r.Until != "" || r.Count > 0 {
		return Interval{}, false, fmt.Errorf("unrecognized interval %q; try 2w, monthly, quarterly, every 45 days or never", s)
	}
	i := Interval{r.every(), freqUnits[r.Freq]}
	if i.Unit == "year" {
		i = Interval{12 * i.Every, "month"}
	}
	return i, false, nil
}
