# Database migration, backup and recovery

## Migration to Phase 3

Phase 3 adds schema migration 3. It runs automatically, once, the first time a Phase 3 build opens a Phase 2 (or Phase 1) database, including with `tidecontacts --check`.

- It only **adds** tables, indexes and the built-in activity types. No existing table, row, preference or history entry is modified.
- The migration and its version record commit in one transaction. If anything fails, the database is left exactly at its previous version.
- A database written by a newer schema than the executable supports is refused, not downgraded.
- There is no downgrade path: a Phase 2 build refuses a migrated database. Back up first if you may want to go back.

New tables: `activities`, `activity_types`, `activity_entities`, `activity_projects`, `activity_tasks`, `activity_tags`, `tasks`, `task_entities`, `task_projects`, `task_tags`, `recurrence_rules`, `reminders`, `projects`, `project_entities`, `project_tags`, `milestones`, `pipelines`, `pipeline_stages`, `pipeline_items`, `pipeline_stage_history`, `item_entities`, `item_projects`, `item_tasks`, `item_activities`, `item_tags`, `relationship_meta`, `followup_prefs`, `important_dates`, `inbox_state`, `collections`, and the full-text table `crm_search`.

Configuration: no setting changed meaning. New commands have default keys (see [keys](keybindings.md)). If your `[keybindings]` already assigns one of those keys to another command, your binding wins and the new command simply has no key; it remains in the palette.

## Where the data is

- Linux: `~/.local/share/tidecontacts/tidecontacts.db` (honours `XDG_DATA_HOME`)
- macOS: `~/Library/Application Support/tidecontacts/tidecontacts.db`
- Windows: the user configuration directory

SQLite runs in WAL mode, so `tidecontacts.db-wal` and `tidecontacts.db-shm` may sit beside the main file while the app is open. All three are created with owner-only permissions.

## Backing up

Either:

1. Quit TideContacts, then copy `tidecontacts.db` (after a clean exit the WAL has been merged). This is the simplest reliable backup.
2. Or, while it is running, use SQLite's online backup, which is consistent:

   ```sh
   sqlite3 ~/.local/share/tidecontacts/tidecontacts.db ".backup '/path/to/backup.db'"
   ```

Do not copy only the main file while the app is open: recent writes may still be in the WAL.

Contact exports (`E`, CSV or vCard) are a portable copy of people and organizations only. They do not contain activities, tasks, projects, pipelines, reminders or collections. The database file is the complete backup.

## Checking and restoring

```sh
tidecontacts --check                                  # opens, migrates if needed, reports paths
sqlite3 backup.db "PRAGMA integrity_check; PRAGMA foreign_key_check;"
```

To restore: quit TideContacts, move the current `tidecontacts.db`, `-wal` and `-shm` files aside, copy the backup to `tidecontacts.db`, and start the app.

## Recovery inside the app

- Contacts are never hard-deleted: search `is:deleted` and run **Restore archived/deleted contact**. `u` undoes the last contact change of the session.
- Tasks, activities, projects and pipeline items are tombstoned on delete (kept in the file, hidden everywhere). There is no in-app undelete for them yet; they can be recovered with `UPDATE … SET deleted=0` in SQLite.
- Every save is one transaction with a revision check. A record changed elsewhere since you opened it is refused with a message instead of being overwritten.
- A merged contact becomes a tombstone naming its successor; its links are not moved back if you restore it.
