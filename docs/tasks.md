# Tasks, recurrence and reminders

## States and views

| State | Tag | Meaning |
|---|---|---|
| inbox | `[?]` | Captured, not yet processed |
| next | `[ ]` | Ready to do |
| scheduled | `[>]` | Has a date |
| waiting | `[w]` | Someone else acts next |
| completed | `[x]` | Done |
| canceled | `[-]` | Dropped |

| View | Shows |
|---|---|
| Inbox | Unprocessed tasks |
| Today | Open tasks due today or earlier (grouped Overdue / Today), and scheduled tasks whose start date has arrived |
| Upcoming | Open tasks with a future due, start or snooze date, grouped by day |
| Waiting | Commitments someone else owes you |
| All | Everything not archived; open work first |
| By person / By project | Open tasks grouped by each linked person or project |
| pinned collections | Any task collection you pin becomes a view here |

`s` cycles the sort (due, priority, title, updated, created). Snoozed tasks leave Inbox, Today and Waiting until the snooze date.

## Actions

| Key | Action |
|---|---|
| `n` / `T` | Quick capture |
| `e` | Full form |
| `c` / `R` | Complete / reopen |
| `S` | Reschedule: `fri`, `+1w`, `2026-12-01 14:30`, `none` |
| `z` | Snooze until a date (default tomorrow) |
| `p` | Cycle priority (`!` low, `!!` medium, `!!!` high) |
| `>` / `<` | Move through inbox → next → scheduled → waiting |
| `L` | Link a person, organization or project |
| `t` | From an activity or inbox entry: create a task from it |
| `y` | Duplicate (same links, no recurrence) |
| `a` | Archive / unarchive |
| `D` | Delete, after confirmation |
| `b` | Add a reminder for the task |

Filters: `status:waiting`, `is:open|done|overdue|today|recurring|waiting|snoozed|unlinked|archived`, `due:today|tomorrow|week|month|overdue|none|any|2026-12-01|<date|>date`, `pri:high`, `with:`, `org:`, `project:`, `tag:`, and free words.

## Recurrence

A rule is written in words, in the form's Repeat field or as `every:` in quick capture:

```text
daily            weekdays              every 3 days
weekly           every 2 weeks         weekly on mon wed
monthly          monthly on 31         quarterly
yearly           every 2 weeks until 2027-01-01
weekly x10       every 2 weeks after completion
```

- **Schedule mode** (default) walks a fixed grid from the first due date. `monthly on 31` falls on the last day of shorter months and returns to the 31st afterwards. A yearly February 29 falls on February 28 in common years.
- **After completion** measures the interval from the day you complete the task.
- `until` and `xN` end the series.

Completing a recurring task creates **exactly one** successor: the next occurrence that is today or later. Missed occurrences are skipped, never back-filled, so a task that is six years overdue still produces one new task. The successor is keyed by (series, occurrence number), so completing twice, or reopening and completing again, cannot create a duplicate. Reopening a task keeps a successor that already exists. Changing the rule on one task changes the series.

### Time zones and daylight saving

Due dates are calendar dates in your local calendar; an optional due time is a wall-clock time. Recurrence works on dates, so a 09:00 task stays at 09:00 on both sides of a daylight-saving change (the day in between is 23 or 25 hours long). A task may name an IANA zone (`Europe/Paris`) for its time; without one the time floats with the machine's zone. A wall-clock time that does not exist on a spring-forward day resolves to an adjacent valid instant that day. "Today" is the local date of the machine's clock.

## Reminders

`b` adds a reminder: `Call the bank on:fri at:09:00`, `Stretch at:11:00 every:day`. Without a date it is set for tomorrow 09:00. A reminder can be linked to the selected task or person. Completing or deleting a task dismisses its reminders.

Insights → Reminders lists them. `c` or `x` dismisses, `z` snoozes to 09:00 on a date, `S` reschedules. Dismissing a recurring reminder moves the same reminder to its next occurrence after now, at the same local time; it never builds a backlog.

**While TideContacts is running**, due reminders are counted in the status bar (checked every minute) and listed first in the People Inbox.

**While it is closed, nothing runs.** TideContacts has no daemon and a closed program cannot notify you. If you want notifications, let the operating system's scheduler start a short-lived process:

- `tidecontacts --due` prints what is due (reminders, due follow-ups, overdue commitments) and exits with status 3 when something is, 0 otherwise.
- `tidecontacts --notify` sends each newly due reminder once to the desktop notification service (`notify-send` on Linux, `osascript` on macOS) and exits.

systemd user timer (Linux):

```ini
# ~/.config/systemd/user/tidecontacts-notify.service
[Service]
Type=oneshot
ExecStart=%h/go/bin/tidecontacts --notify

# ~/.config/systemd/user/tidecontacts-notify.timer
[Timer]
OnCalendar=*:0/15
Persistent=true
[Install]
WantedBy=timers.target
```

Enable with `systemctl --user enable --now tidecontacts-notify.timer`. With cron: `*/15 * * * * tidecontacts --notify`. On macOS use a launchd agent with `StartInterval`. This is optional, off by default, and limited by your scheduler: reminders are as punctual as the timer you choose. Windows has `--due` only.
