# People Inbox

Insights → Inbox (workspace 8, or **Review People Inbox** in the palette) is a review queue, not a feed. It holds only things a rule says need your attention now, one entry per source record.

## What appears

| Entry | Rule |
|---|---|
| Reminder due | A pending reminder whose time has passed |
| Overdue commitment | An open, unsnoozed task whose due date is before today |
| Follow-up due | A follow-up date you set has arrived |
| Suggested follow-up | A contact's cadence has elapsed since the last recorded interaction |
| Waiting on someone | A task in the waiting state, untouched for the configured number of days (default 7) |
| Unprocessed task | A task still in the inbox state |
| Important date | A birthday, anniversary or custom date within the configured window (default 14 days) |
| Flagged for review | A contact you flagged with `!` |

A task appears once: overdue beats waiting beats unprocessed. Entries are grouped Overdue, Due now, Upcoming and unprocessed, then Reviewed.

Every entry shows **why it is here**, its source record, the related person, the date, and when it first appeared. Suggested follow-ups are deterministic: they come only from a cadence you configured and interactions you recorded.

## Acting

| Key | Action |
|---|---|
| Enter | Open the source record (Esc comes back) |
| `c` | Complete: finish the task, dismiss the reminder, clear the flag, or — for a follow-up — log the interaction that happened |
| `t` | Open the task, or create one for the person |
| `z` | Snooze until a date |
| `x` | Dismiss |
| `v` | Mark reviewed (stays listed, dimmed, at the bottom) or un-review |
| `S` | Change the schedule: task due date or follow-up cadence |
| `F` `A` `T` | Follow-up schedule, log activity, new task for the person |

## Dismissal and reappearance

Each entry carries a fingerprint of the facts behind it (for a task: kind, due date and state; for a follow-up: due date and last interaction; for a date: this year's occurrence; for a reminder: its time).

- **Dismissed** entries stay hidden for as long as the fingerprint is unchanged, however much time passes. They return only when the facts change: the task gets a new due date, a newer interaction moves the cadence, next year's birthday approaches.
- **Snoozed** entries return on the chosen date.
- **Reviewed** entries stay visible but no longer sort among open work.
- An entry disappears by itself when its rule stops matching (the task is completed, the reminder dismissed).

Remembered state for records that stopped qualifying is cleared after 180 days.

The same due items are available to scripts with `tidecontacts --due`; see [tasks](tasks.md#reminders).
