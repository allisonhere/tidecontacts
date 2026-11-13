# Relationship Radar

Insights → Radar is a set of deterministic signals about your relationships. It is not AI and it does not score anyone. Each row is one person and one sentence stating the recorded fact behind it. No signal says, or implies, that a person is unimportant or neglected.

| Signal | Shows | Reason looks like |
|---|---|---|
| Due for follow-up | Contacts whose configured follow-up is due (optionally a few days early) | "Monthly cadence; last recorded call on 2026-10-01 (43 days ago); due 3 days ago" |
| Open commitments | People linked to open tasks | "3 open tasks (1 waiting on them); earliest due 2026-11-20" |
| Upcoming important dates | Birthdays, anniversaries and custom dates in the window | "Birthday on 2026-11-20 (in 7 days)" |
| Configured contact intervals | Contacts with a cadence that is not yet due | "Monthly cadence; last recorded meeting on 2026-11-02 (11 days ago); next due 2026-12-02" |
| Recently active collaborators | People with at least N recorded interactions in the window | "5 interactions recorded in the last 30 days; latest on 2026-11-10" |
| Linked to overdue work | People linked to an overdue open task or to an open project past its target date | "Linked to overdue task “Send proposal” (due 2026-11-01)" |

`Enter` opens the person. `F`, `A`, `T`, `z` act on them as elsewhere.

## What it deliberately does not do

- It never infers anything from the absence of data. A person with no recorded interactions and no cadence appears nowhere. An empty history usually means nothing was imported.
- It has no health score, ranking, sentiment, or "at risk" label.
- It reads only local records: activities you logged or imported, tasks, projects, dates and schedules.

## Settings

Run **Relationship Radar settings** from the palette. Every signal has an on/off switch, and these thresholds are adjustable:

| Setting | Default | Used by |
|---|---|---|
| Important-date window (days) | 14 | Dates signal and People Inbox |
| Recently-active window (days) | 30 | Active signal |
| Interactions needed to count as active | 2 | Active signal |
| Show follow-ups this many days early | 0 | Follow-up signal |
| Waiting tasks surface in the inbox after (days) | 7 | People Inbox |
| Rows per signal | 25 | All signals |

Settings are stored in the local database. Out-of-range values fall back to the defaults.
