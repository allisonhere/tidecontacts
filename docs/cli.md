# Scripting TideContacts

The CLI reads and writes the same SQLite records as the UI and TideMail plugin.
Neither application needs to be running. No server, account or network service is
required. Running `tidecontacts` with no subcommand still opens the terminal app.
The examples below use `./bin/tidecontacts`; an installed binary works the same way.

## Start with demo data

```sh
./bin/tidecontacts --demo contacts list
./bin/tidecontacts contacts list --demo --full --query 'tag:demo'
./bin/tidecontacts --demo tasks list --view today
./bin/tidecontacts schema contacts
./bin/tidecontacts help
```

`--demo` uses the separate, persistent demo database, seeded on first use. It does
not open the real database. `--database /absolute/path/to/test.db` selects another
explicit database and cannot be combined with `--demo`. Without either option,
the CLI uses your normal TideContacts database. Commands initialize or upgrade
that selected database just like the app; `schema` and `help` do not open a database.
There is no interactive confirmation or terminal rendering on the command path.

## JSON contract

Successful data commands write one JSON object to stdout:

```json
{
  "schema_version": 1,
  "data": {"ID": "...", "Revision": 1, "Title": "Send proposal"},
  "meta": {"resource": "tasks", "action": "create", "count": 1, "possibly_truncated": false}
}
```

List commands put an array in `data`. Contact lists contain summaries by default;
use `--full` for email addresses, methods, notes and other aggregate fields. Full
records include private notes, as the app's exports do. Search index internals
are omitted from summaries. IDs are stable strings; use them rather than names
for subsequent operations and links.

Record fields retain the existing domain/export spelling, including `ID`,
`Revision`, `DisplayName`, `Title`, `Entities` and `Stages`. Use `schema RESOURCE`
to inspect fields, nested types and supported actions. Exact field names are
required at the top level. Nested reference/rule fields keep their existing JSON
tags, such as `id`, `role`, `freq` and `by_day`; `schema` reports those names. The outer CLI envelope uses snake_case. This is API
version 1; incompatible envelope or record changes require a version change.
`--json` is optional because JSON is already the default. `help` / `--help` is
human-readable text. Existing flags such as `--check`, `--due`, `--notify` and
`--sync-tidemail` retain their original output and behavior.

Errors produce no normal stdout and write a versioned JSON diagnostic to stderr:

```json
{"schema_version":1,"error":{"code":"conflict","message":"revision changed; get the record again"}}
```

| Exit code | Meaning |
|---|---|
| 0 | Success |
| 1 | Runtime failure, such as I/O, database locking or a timeout |
| 2 | Unknown command/option, missing argument or other usage error |
| 3 | Invalid JSON, invalid record, unsupported field or read-only external activity |
| 4 | ID not found |
| 5 | Revision or pipeline configuration conflict |

OS signals keep their normal process-termination behavior. Database operations have a
five-minute context deadline. Scripts should check the exit code before consuming
stdout. Database writes use the app's normal transactions and validation.

## Create, read and update

JSON input is explicit: `--input FILE` or `--input -` for stdin. One command accepts
one JSON object, up to 50 MiB. Unknown fields, top-level null record fields and trailing JSON
values are rejected. Use empty strings, empty arrays or false to clear fields.
Create requests omit IDs, revisions and creation/update timestamps.

```sh
printf '%s\n' '{"DisplayName":"Ada Example","Methods":[{"Kind":"email","Value":"ada@example.com"}]}' |
  ./bin/tidecontacts --demo contacts create --input -

printf '%s\n' '{"Title":"Send proposal","Due":"2026-12-01","Priority":3}' |
  ./bin/tidecontacts --demo tasks create --input -

./bin/tidecontacts --demo contacts get CONTACT_ID
./bin/tidecontacts --demo tasks get TASK_ID
```

Updates are partial at the top level. Omitted fields are preserved; supplied
arrays and nested objects replace that whole field. Preserve existing array
entries and IDs if you want to retain them. For example, replacing `Methods`
replaces the entire methods collection. IDs, record kind and original timestamps
cannot change. Record updates require the current `Revision`:

```sh
./bin/tidecontacts --demo tasks get TASK_ID |
  jq '.data | .Title = "Updated proposal"' |
  ./bin/tidecontacts --demo tasks update TASK_ID --input -
```

A smaller patch can contain just `Revision` and the changed fields. Stale revisions
return exit 5; reload and decide how to apply your change. A normal UI, plugin or
other CLI edit cannot be silently overwritten by a stale record update. Pass the
record's `.data`, not the CLI envelope, as update input.

## Supported operations

| Resource | Actions |
|---|---|
| `contacts`, `organizations` | list, get, create, update, delete, restore, history, dossier, connections |
| `tasks` | list, get, create, update, delete, restore, complete, reopen, duplicate |
| `activities` | list, get, create, update, delete, restore |
| `projects` | list, get, create, update, delete, restore |
| `pipelines` | list, get, create, update, archive |
| `items` | list, get, create, update, delete, restore, move |
| `crm` | export, import |

`organizations` fixes Kind to organization. `contacts list --kind organization`
is an alternative. Activity creation is for local entries; connector-owned
activities remain read-only. Pipelines archive rather than delete, matching the
UI. New stages and milestones omit IDs; a pipeline update may retain only stage
IDs already belonging to that pipeline.

Delete, restore and pipeline archive actions require `--yes` and use recoverable
tombstones or archive flags; they do not hard-delete records. Mutations other than
record update act on the current stored record, matching the corresponding UI
command. Duplicate intentionally creates another task each time it is called.

```sh
./bin/tidecontacts --demo tasks complete TASK_ID
./bin/tidecontacts --demo tasks reopen TASK_ID
./bin/tidecontacts --demo contacts delete CONTACT_ID --yes
./bin/tidecontacts --demo contacts restore CONTACT_ID --yes
```

## Links and pipelines

Link records using ID references in their normal JSON fields:

```json
{"Title":"Call Ada","Entities":[{"id":"CONTACT_ID"}],"Projects":[{"id":"PROJECT_ID"}]}
```

Pipeline configurations do not have numeric revisions. A `pipelines get` response
includes `.etag`, computed from its stored configuration and stage order, excluding
derived item counts. Pipeline update/archive requires `--if-match ETAG`. The old
configuration is checked again inside the write transaction, so a competing edit
cannot be overwritten after the CLI's initial read. Stage removal rules are the
same as in the app: a stage containing items cannot simply be removed.

```sh
printf '%s\n' '{"Name":"Opportunities","Stages":[{"Name":"New"},{"Name":"Done"}]}' |
  ./bin/tidecontacts --demo pipelines create --input -

./bin/tidecontacts --demo pipelines get PIPELINE_ID
printf '%s\n' '{"Name":"Renamed opportunities"}' |
  ./bin/tidecontacts --demo pipelines update PIPELINE_ID --if-match ETAG --input -

./bin/tidecontacts --demo items move ITEM_ID --stage STAGE_ID
./bin/tidecontacts --demo pipelines archive PIPELINE_ID --if-match ETAG --yes
```

## Lists, filters and completeness

Use the same query grammar as the app, for example `tag:friend`,
`is:deleted`, `type:meeting` or `is:open due:today`, on the corresponding resource.
`pipelines list` includes archived configurations and does not accept `--query`.
Task views are all, inbox, today, upcoming, waiting and open. `--entity ID` limits
task/activity lists to a person or organization; `--pipeline ID` limits items.
Projects accept `--status all|planned|active|on-hold|completed|archived`.
An option irrelevant to a command is rejected rather than ignored.

Task/activity lists default to 200 records. `--limit` accepts up to 5,000 tasks or
1,000 activities. Project lists have a 2,000-record cap and item lists a 5,000-record
cap. Responses include `meta.limit` for capped lists and set
`meta.possibly_truncated` when that cap is reached. This is conservative: an exact
cap-sized result might be complete. There is no cursor pagination yet. Contact
lists and pipeline configurations are not capped. For a complete CRM snapshot,
use `crm export`, which is not constrained by interactive list caps.

## Archive workflows

```sh
./bin/tidecontacts --demo crm export | jq '.data' > crm.json
./bin/tidecontacts --demo crm import --input crm.json
```

This is the existing versioned CRM archive format. It excludes contact aggregates;
links refer to contact IDs. Import adds missing records in one transaction and
never overwrites existing ones. Missing contact links are reported by the normal
import report. Contact CSV/vCard transfer and batch contact restore are still the
existing UI workflows. Reminders, insights, settings and connector actions retain
their existing UI/legacy CLI entry points; this first command API focuses on the
core records listed above.

## Verification

`make smoke-cli` drives the actual binary without a TTY against an isolated demo
profile. Unit tests cover every core resource, partial updates, links, duplicate
commands, invalid JSON, revision conflicts, pipeline tokens, archive handling,
external-activity ownership and profile isolation.

## Mail-client recipient lookup

`contacts query [TEXT]` provides the minimal email/name/organization projection.
It defaults to JSON and can use the aerc or NeoMutt wire format with `--format`.
This lookup treats text literally, searches both people and organizations, and
opens real databases read-only. See [mail-client setup](mail-clients.md) for
format-specific exit behavior, limits and the aerc query-substitution limitation.

## Connected-folder contacts

Contacts owned by a connected vCard directory are read-only through these commands too. A supplied replacement `Source`, revision, delete or restore cannot turn one into a local editable contact. Edit the source vCard and refresh through TideContacts or TideMail. Directory connection management currently uses the import modal; it has no dedicated JSON resource.

`contacts query` reads cached folder contacts without refreshing or migrating. Open the updated app or run `--check` if an older database needs schema 4, then refresh connected folders in the app when the source has changed. See [connected folders](vcard-directories.md).
