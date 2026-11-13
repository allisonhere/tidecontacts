# Smart collections

A smart collection is a named, saved filter. Only the filter text is stored, so a collection is always current: it is evaluated against live data each time it is shown.

## Creating

Filter any workspace with `/`, then press `C` (or run **Save current filter as a smart collection**) and name it. The record type comes from the workspace, the filter from the search line, and the sort from the current sort. In Tasks, the current view is included (Inbox becomes `status:inbox`, Waiting becomes `status:waiting`, and so on).

Insights → Collections lists them. Selecting one previews its results; `Enter` opens it in its workspace with the filter applied (Esc returns). `e` edits name, description, type, filter, sort, grouping and pinned state; `f` pins or unpins; `n` creates one from scratch; `D` deletes the saved filter (never the records). Pinned task collections also appear as extra views in Tasks, which is how you add your own task views.

## Examples

| Collection | Type | Filter |
|---|---|---|
| People I need to contact this week | person | `followup:week` |
| Open tasks involving Project Harbor | task | `is:open project:Harbor` |
| Active collaborators at Riverside | person | `org:Riverside seen:30d` |
| Contacts with upcoming birthdays | person | `birthday:month` |
| Projects waiting on external responses | project | `is:waiting` |
| Calls with Alex this quarter | activity | `type:call with:alex after:2026-10-01` |
| Stalled applications | item | `pipeline:applications stage:applied` |

## Fields

| Field | Values |
|---|---|
| Record type | person, organization, task, activity, project, item |
| Filter | The [search grammar](search.md) of that record type |
| Sort | People: name, organization, updated, added. Tasks: due, priority, title, updated, created |
| Group by | Tasks: status, due, priority, person, project. People: org. Others use their natural grouping (day, status, stage) |
| Pinned | Sorts first; pinned task collections become Tasks views |

A filter is validated when the collection is saved. Collections use the same grammar as interactive search; there is no second query language.
