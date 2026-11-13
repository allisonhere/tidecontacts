# Personal CRM guide

TideContacts is a personal CRM: people, relationships and commitments come first. Pipelines exist but are optional, and nobody has to be a "lead". Everything below works offline, with no account, TideMail, plugin or AI.

## The eight workspaces

| Key | Workspace | What it is for |
|---|---|---|
| 1 | People | Contact profiles, plus each person's **Dossier** and **Timeline** views |
| 2 | Organizations | Companies and teams, with the same dossier |
| 3 | Relationships | Explorer: tree, connected people, organizations, projects, profile |
| 4 | Activities | One timeline of calls, meetings, notes, emails and custom entries |
| 5 | Tasks | Inbox, Today, Upcoming, Waiting, All, By person, By project, pinned collections |
| 6 | Projects | Overview, People, Tasks, Timeline, Milestones, Notes |
| 7 | Pipelines | Keyboard-driven boards with stages you define |
| 8 | Insights | People Inbox, Relationship Radar, Smart collections, Reminders, Follow-ups |

Every workspace has the same shape: a list on the left, details in the middle, context and key hints on the right (wide terminals). `[` and `]` change the view, `/` filters with one shared search grammar, `Enter` opens or follows, `Esc` goes back.

## Quick capture

Two keys work from anywhere and link to whatever is selected (a person, an organization or a project):

- `T` — new task
- `A` — log an activity (also **Log meeting**, **Log call** and **Add note** in the palette)

Type one line and press Enter. Bare words are the title; `key:value` pairs set fields using the same words as search filters:

```text
Send proposal with:"Alex Rivera" due:fri pri:high project:Harbor
Coffee type:meeting on:yesterday at:10:30 tag:catchup
Water plants due:tomorrow every:week
```

| Key | Meaning |
|---|---|
| `with:` `org:` | Link a person or organization by exact unique name |
| `project:` `tag:` | Link a project, add a tag |
| `due:` `start:` `on:` | today, tomorrow, fri, +3d, +2w, +1m, 2026-12-01, 12-01 |
| `at:` | 24-hour time, `14:30` |
| `pri:` | none, low, medium, high |
| `every:` | A recurrence: `week`, `"2 weeks"`, `weekdays`, `"monthly on 31"` |
| `type:` `status:` | Activity type; task status |

Unknown `word:` pairs such as "Re:" stay in the title. A capture that cannot be understood stays in the prompt with the reason and writes nothing. Tasks captured without a date go to the task Inbox for later processing. Press `e` on any record for the full form (Tab between fields, ←/→ on choices, a multi-line editor for notes, F2 to save).

## The contact dossier

In People or Organizations press `[` once from Overview (or `]` to walk forward) to reach:

- **Dossier** — follow-up schedule with its reason, open tasks, recent commitments (waiting on them, or finished in the last 30 days), projects, connections and important dates.
- **Timeline** — the latest twelve recorded activities, with a link to the full, paged timeline.

With the profile focused, `j/k` move between linked records and `Enter` opens one. `Esc` returns to the same person, view and cursor, however many links deep you went. From the profile: `T` task, `A` activity, `F` follow-up, `d` important date, `!` flag for review, `b` reminder, `c` completes the task under the cursor.

## Follow-ups

`F` on a person accepts a cadence (`2w`, `monthly`, `quarterly`, `every 45 days`), a one-off date (`fri`, `+3d`, `2026-12-01`), `never`, or `none` to clear.

A cadence is measured from the **last recorded interaction** with that person: any activity except notes, task completions and project updates, dated today or earlier. If nothing is recorded, it is measured from the day you set the cadence. With no cadence and no date, nothing is ever due: TideContacts does not treat missing history as neglect, because missing history usually means nothing was imported. Every due follow-up states its reason, for example *"Monthly cadence; last recorded call on 2026-10-01 (43 days ago)"*. Logging an interaction moves the cadence forward and settles an explicit date that has arrived.

## Relationships

Workspace 3 lists people and organizations; the middle pane explores the selected one:

- **Tree** — outgoing and incoming relationships, memberships and projects, two levels deep, drawn with line characters.
- **People** — everyone connected, with the reasons (relationship, shared organization, shared project, shared activities).
- **Organizations**, **Projects** — memberships and collaborators.
- **Profile** — relationship labels and notes, follow-up, important dates and recent history.

`Enter` on a list row focuses the explorer; `Enter` on a node re-roots the explorer on that person (Esc returns). `r` adds a relationship using a searchable picker and any label you like, `x` removes the one under the cursor, `e` opens the full contact profile, and **Edit relationship notes** in the palette records how you know someone.

## Merging duplicates

In People, mark the duplicate with Space, select the record to keep, and run **Merge marked contact into selected contact** from the palette. After confirmation, methods, tags, notes, memberships, relationships and every task, activity, project, pipeline item, reminder, date and follow-up schedule move to the kept record. The duplicate becomes a tombstone that names its successor. Restoring it later does not move the links back.

## Privacy

All data stays in one local SQLite file with owner-only permissions. There is no telemetry and no network code. The only things that leave the process are files you export, links you open with `o`, and desktop notifications you schedule yourself with `--notify`. Routine logs contain no contact names, task titles or notes.

See also: [activities](activities.md), [tasks and recurrence](tasks.md), [projects and pipelines](projects-pipelines.md), [People Inbox](inbox.md), [Relationship Radar](radar.md), [smart collections](collections.md), [keys](keybindings.md), [migration and backup](migration-backup.md).
