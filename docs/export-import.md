# CRM export, import and restore

Contacts keep their own CSV/vCard export (`E`, `I`; see [import and export](import-export.md)). Everything else the CRM stores travels in a second, versioned format. Everything here is local; nothing is sent anywhere.

## Commands (palette, `:`)

| Command | What it does |
|---|---|
| Export CRM records (JSON) | Writes one `.json` file |
| Export tasks and reminders (iCalendar) | Writes one `.ics` file |
| Import CRM records (JSON) | Reads a `.json` export, shows what it contains, asks, then imports |

Imports open the shared **Import data…** modal, preselected to CRM archives. Browse to a `.json` file or press `p` to enter its path (`~/` works). Exports ask for a destination path. Exports go through a temporary file and are published atomically; an existing destination is never replaced and a failure leaves nothing behind. The JSON file contains your private notes: treat it like the database.

## What the JSON export contains

Activities (with provenance, source references and links), tasks (with recurrence), reminders (pending), projects (members, roles, milestones, tags), pipelines (stages, items, links, full stage history, archived items), follow-up schedules, important dates, relationship notes and review flags, and smart collections. It does **not** contain contacts (use the contact export), deleted records, dismissed reminders, inbox dismissals or radar settings. Links refer to contacts and other records by ID.

The file starts with `"format": "tidecontacts-crm"` and a `"version"`. A build refuses a version it does not know rather than guessing.

## Importing

1. The file is read and checked; a wrong format or unsupported version is reported.
2. A confirmation shows the counts it contains. Nothing has been written yet.
3. On `y`, everything is added in **one transaction**: any failure rolls back the whole import.

Rules:

- **Nothing existing is overwritten or changed.** A record whose ID is already present is skipped and counted ("already present"), so importing the same file twice is harmless. External activities are also matched by source application and reference.
- **Links to contacts that are not in this database are dropped** and listed as warnings (the first is shown in the status line); the record itself is still imported. CSV/vCard contact imports create new local IDs, so importing a contact export first does not preserve archive links to the old IDs. Use the original database or a full database backup when those identities and links must be preserved.
- Pipelines are imported whole or skipped whole; stages and item history keep their original names and dates.
- Timestamps of creation and last change become the import time. Activity times, due dates, completion times and stage-entry times are preserved.
- Follow-up schedules, dates, relationship notes and collections are added only where none exist for that contact or name.

## iCalendar

Export only; iCalendar cannot be imported. Tasks become `VTODO` (due date or date-time with optional `TZID`, status, priority, categories, `RRULE`), reminders become `VEVENT` with a display alarm at the event time, and people linked to a task are listed in `X-TIDECONTACTS-WITH`. Differences from TideContacts: only the open occurrence of a recurring task carries the `RRULE`; `monthly on 31` follows the standard (short months are skipped instead of clamped); `after completion` rules cannot be expressed and appear as an `X-TIDECONTACTS-REPEAT` note on a one-off item.

## Restore

Deleting a task, activity, project or pipeline item keeps a tombstone in the database. To bring one back, open its workspace, filter with `is:deleted`, select it and run **Restore archived/deleted contact** from the palette. Restoring also takes the record out of the archive (an archived project returns as active), so it reappears in the normal lists. Restoring twice is harmless. Reminders that were dismissed when a task was deleted stay dismissed. A deleted pipeline item returns to its original stage. Contacts keep their own restore flow, and `u` undo covers contacts only.
