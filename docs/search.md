# Search grammar

A query is a sequence of whitespace-delimited terms or `key:value` filters. Double quotes allow spaces in a value or term. A backslash escapes the next character. Matching is case insensitive. Filters and terms are combined with AND; OR, negation and arbitrary SQL/FTS syntax are not exposed.

Examples:

```text
alex
"Alex Rivera"
org:"Riverside Studio" tag:developer
is:favorite has:phone
@github.com
+12345678900
```

| Filter | Meaning |
|---|---|
| tag:value | Exact tag name, case insensitive |
| org:value | Organization name contains value |
| is:favorite | Favorited records |
| is:archived | Archived records instead of active records |
| is:active | Explicitly active records |
| is:deleted | Recoverable tombstones; add is:archived for archived tombstones |
| has:kind | Has an email, phone, website, social, messaging or address method |

Unclosed quotes, trailing escapes, unknown filters and empty values produce inline errors. Existing results remain visible until the syntax is fixed or cleared; no new result replaces them on error. Quoted terms are safely escaped before FTS5 use, and filters use bound SQL parameters.

Search documents contain display and structured names, aliases, nicknames, phonetic names, methods, normalized phone/email values, address components, organization memberships/titles, tags, biography and private notes. FTS5 uses Unicode tokenization with accent removal and prefix matches. Normalized exact/substring matching handles punctuation in phone numbers and email domains. Fuzzy ranking supplements this over names, organizations and tags.

FTS candidates rank first, followed by normalized/substring and fuzzy matches. The chosen sort breaks ranking ties; without terms it directly controls order. IDs provide deterministic final ties. Compact summaries are read in the background; only visible list rows render. The fuzzy fallback makes one linear summary/document pass, not a scan per contact. Search is debounced for 100 ms; stale/canceled results are ignored.

## One grammar, six record types

Phase 3 uses this same lexer and AND-only grammar everywhere: People, Organizations, Relationships, Activities, Tasks, Projects, Pipelines, smart collections and quick capture. Only the filter vocabulary depends on the record type. An unknown filter names the ones that are valid there.

### Contacts (additions)

| Filter | Meaning |
|---|---|
| is:flagged | Flagged for review |
| has:tasks / has:followup / has:dates | Has open tasks / a follow-up schedule / a birthday, anniversary or custom date |
| followup:due / week / month / set / none | Follow-up due now / within 7 days / within a month / any schedule / no schedule |
| date:today / week / month | Any important date in the window |
| birthday:today / week / month | Birthday in the window |
| project:value | Collaborator on a project whose name contains value |
| seen:30d / 8w / 6m / never | A recorded interaction within the window / none recorded |

### Tasks

`status:` (inbox, next, scheduled, waiting, completed, canceled) · `is:` (open, done, overdue, today, recurring, waiting, snoozed, unlinked, archived) · `due:` (today, tomorrow, a weekday, +3d, week, month, overdue, none, any, YYYY-MM-DD, `<date`, `>date`) · `pri:` (none, low, medium, high) · `with:` · `org:` · `project:` · `tag:`

### Activities

`type:` · `with:` · `org:` · `project:` · `tag:` · `after:` · `before:` · `on:` · `source:` (local, imported, external) · `app:`

### Projects

`status:` (planned, active, on-hold, completed, archived) · `is:` (open, overdue, waiting, archived) · `pri:` · `with:` · `org:` · `tag:`

### Pipeline items

`pipeline:` · `stage:` · `with:` · `org:` · `project:` · `tag:` · `is:` (open, archived)

`with:` and `org:` match names by substring (or an exact ID); `org:` also matches people who are members of that organization. `tag:` is an exact, case-insensitive tag. Dates are evaluated in your local calendar. Free words use FTS5 prefix matching over titles, text and linked names, plus a substring match on the title. Insights lists (Inbox, Radar, Collections, Reminders, Follow-ups) filter by plain words only.

### Quick capture

Capture lines use the same tokens with assignment meaning: `with:` `org:` `project:` `tag:` `type:` `status:` `stage:` `pri:` `due:` `start:` `on:` `at:` `every:`. Unlike search, an unrecognized `word:` is kept as title text.
