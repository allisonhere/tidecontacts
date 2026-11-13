# Activities and the timeline

An activity is something that happened: an email reference, call, meeting, conversation, message, note, event, task completion or project update. Any other single word is a custom type (`type:standup`). An activity can involve several people and organizations, several projects and several tasks at once.

## Timeline

Workspace 4 shows activities newest first, grouped by day. Each row has the time, a four-letter type tag (`mtg`, `call`, `note`, `mail`, `msg`, `conv`, `evnt`, `done`, `proj`, or the first letters of a custom type), the title and the people involved. `↗ app` marks a reference to a record owned by another application; `⇣ app` marks an imported copy.

`[` / `]` switch between All and one type. `/` filters:

| Filter | Meaning |
|---|---|
| `type:call` | One type, built-in or custom |
| `with:alex` `org:riverside` | Involves a person or organization (name contains) |
| `project:harbor` `tag:planning` | Linked project, exact tag |
| `after:2026-11-01` `before:fri` `on:yesterday` | Date range in your local calendar |
| `source:local` `source:imported` `source:external` `app:tidemail` | Provenance |
| any other words | Full-text search of titles, notes and linked names |

The list loads 200 entries at a time; the last row loads older ones. A person's profile loads only the latest twelve and links to the full timeline. Nothing appears on a timeline unless you logged it, imported it, or a task or project produced it. There is no invented history.

## Recording and editing

`n` or `A` logs with quick capture; `e` opens the form (type, title, when, people, projects, tags, visibility, multi-line notes). `t` creates a follow-up task carrying the same people and projects and a link back to the activity. `L` links more people or projects; in the detail pane `x` unlinks the row under the cursor. `D` deletes after confirmation.

Completing a task that is linked to a person, organization or project adds one `done` entry; reopening the task withdraws it. Changing a project's status adds a `proj` entry.

## Provenance

| Provenance | Meaning | Editable here |
|---|---|---|
| local | Created in TideContacts | Yes |
| imported | Copied in from another tool, with its source and reference kept | Links only |
| external | A reference to a record that lives elsewhere | Links only |

Imported and external activities are read-only: you can link them to people, projects and tasks and delete them from your timeline, but the content belongs to the source. `o` opens an external `http(s)` or `mailto` link with the system opener; other references are shown, not opened.

### For integrations

`UpsertExternalActivity` is the only write path for connectors. A record is identified by `(source application, external reference)`; repeating a call updates the same row. Links supplied by the source are added to links you made locally and never remove them. Participants given as email addresses are linked when exactly one live contact owns that address. Nothing is deleted because a source is unavailable: there is no "sync and prune" operation. No connector ships in this phase; TideMail's message IDs and addresses fit this interface.

## Time

The instant is stored in UTC together with the zone it was recorded in. The timeline groups by your current local day; the detail pane names the original zone when it differs.
