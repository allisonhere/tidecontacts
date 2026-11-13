# Keyboard controls

Every key below is a command in one registry: the same command runs from its key or from the palette (`:` or Ctrl+K), and `?` lists them all. Keys are contextual: `n`, `e`, `a`, `D`, `c`, `x` and the others act on the selected record of the active workspace. Text inputs, prompts and forms own every printable key while they are open.

## Browser

| Keys | Action |
|---|---|
| j/k, Down/Up | Move contact selection; scroll a focused profile |
| PageDown/PageUp, Home/End | Page or jump within list/profile |
| h/l, Left/Right, Tab/Shift+Tab | Move pane focus |
| Enter | Focus selected contact's profile |
| Esc | Go back to where a followed link came from; otherwise return to the list; clear active search; close overlay |
| / | Incremental workspace search |
| :, Ctrl+K | Fuzzy command palette |
| ? | Contextual help; j/k scroll, Esc close |
| 1–8 | Switch workspace: People, Organizations, Relationships, Activities, Tasks, Projects, Pipelines, Insights |
| n / e | New / edit contact or organization |
| f / a | Toggle favorite / archived |
| D | Delete with y/n confirmation |
| u | Undo last recoverable change in this session |
| [ / ] | Previous/next profile view (… History, Dossier, Timeline) |
| s | Cycle name/organization/updated/added sort |
| Space | Mark/unmark contact for export |
| I / E | Import / export filtered records |
| Shift+Left/Right | Shrink/grow left pane |
| Shift+Down/Up | Shrink/grow context pane |
| q | Quit outside inputs/overlays |
| Ctrl+C | Safe exit; cancels an active save/import before exiting |
| Ctrl+Z | Suspend outside the editor |

The palette also exposes restore, export-all, export-marked/selected, density and three theme choices. Normal lists exclude archived and deleted records. Search `is:archived` or `is:deleted` to recover them. Incoming relationships and organization members appear in profiles; edit the source person/contact to change those links.

## CRM workspaces (3–8)

| Keys | Action |
|---|---|
| j/k, Down/Up, PageDown/PageUp, Home/End | Move in the list; with the detail pane focused, move between linked records |
| Enter | Focus details; on a linked record, open it; on an inbox entry, signal or collection, open its source |
| Esc | Back to the record you came from, then to the list |
| [ / ] | Previous/next view (list view, detail section, or pipeline) |
| / | Filter with the workspace's [search grammar](search.md) |
| n / e | New record (quick capture or form) / edit in the full form |
| T / A | Quick-capture a task / log an activity, linked to the selection — works in every workspace |
| b | Add a reminder (linked to the selected task or person) |
| c / R | Complete (task, milestone, reminder, inbox entry) / reopen task |
| S / z | Reschedule (due date, target date, milestone date, reminder) / snooze |
| p | Cycle priority (task, project) |
| > / < | Advance / move back: task state, project status, pipeline stage |
| L | Link a person, organization or project through a searchable picker |
| x | Dismiss inbox entry or reminder; unlink the row under the detail cursor; remove milestone, date or relationship |
| v | Mark inbox entry reviewed / un-review |
| t | Create a task from the selected activity, inbox entry or pipeline item |
| y | Duplicate task |
| o | Open an activity's external http(s)/mailto link |
| M | Add project milestone |
| F / d / ! / r | For the selected person: follow-up schedule / important date / review flag / new relationship |
| C | Save the current filter as a smart collection |
| f | Pin or unpin a collection |
| s | Tasks: cycle sort. Projects: cycle status filter |
| a / D | Archive / delete with confirmation |

Pipelines board: ←/→ or h/l change stage, ↑/↓ or j/k change item, `>`/`<` move the item, Enter opens it. See [projects and pipelines](projects-pipelines.md).

Palette-only commands: Log meeting, Log call, Add note, New project, Open project…, New pipeline, New pipeline item, Rename pipeline / edit stages, Archive or unarchive pipeline, Move pipeline up/down, Move item to stage…, Add / Rename / Remove / Reorder pipeline stage, Explore relationships of selected contact, Edit relationship notes, Review People Inbox, Open Relationship Radar, Relationship Radar settings, Open smart collections, Open reminders, Open follow-up schedules, Merge marked contact into selected contact.

### Prompts, pickers and forms

| Where | Keys |
|---|---|
| Quick-capture prompt | Type, Enter accepts, Esc cancels. Errors keep the prompt open |
| Picker | Type to search, Up/Down (or Ctrl+P/Ctrl+N) select, Enter chooses, Esc cancels |
| Confirmation | y confirms, n or Esc cancels |
| Record form | Tab/Shift+Tab or Up/Down move between fields, Left/Right/Space change a choice, F2 or Ctrl+S saves, Esc cancels (asks before discarding changes) |
| Multi-line field in a form | Same editor as contact notes; Tab leaves it |

## Full-screen editor

| Keys | Action |
|---|---|
| F5 / F6 | Previous/next section |
| Tab / Shift+Tab | Previous/next field; switch Notes/Biography when editing multiline text |
| Ctrl+N / Ctrl+D | Add/remove a method, membership, custom field or relationship |
| F2 / Ctrl+S | Validate and atomically save |
| Esc | Cancel; confirm discard if modified |
| Ctrl+C | Exit; confirm discard if modified |

All printable keys belong to the focused input. Repeated fields remain on one editor surface rather than opening modal chains. Flags accept true/false. Tags, aliases and enumerated choices use comma-separated values (CSV quoting supports commas inside values). Custom field names reuse a global definition with a fixed type/options; changing an existing global definition to a conflicting type is rejected.

Ripple notes/biography: arrows/Home/End/Page keys move, Shift+movement selects, Ctrl+A selects all, Ctrl+Z/Ctrl+Y undo/redo, Alt+C copies, Ctrl+X cuts, Ctrl+V pastes when OS clipboard helpers are available. Terminal bracketed paste works independently. F2 saves the record; Esc belongs to the host's cancel flow. The notes editor's own Find command is disabled so its Tab/Esc rules cannot conflict with the full-screen form.

## Imports/exports

Enter accepts the path. CSV mapping: Up/Down selects columns, Left/Right cycles mappings, h toggles header handling. Preview: Up/Down moves, Space includes/excludes valid rows, b returns to mapping, Enter opens confirmation. y confirms; n returns. Esc cancels before writing. During a transaction, Ctrl+C requests cancellation; a final success or rollback precedes exit. Export destinations must not exist.

## Customization

`[keybindings]` maps command IDs to replacement arrays. An empty array removes the normal shortcut but retains the palette command. Conflicts, reserved keys, unknown commands and invalid key names fail configuration validation. Input editing/overlay controls are fixed; browser bindings are configurable. Existing Phase 1 command IDs remain supported.

Phase 2 IDs: new-contact, edit-contact, favorite, archive, restore, delete-contact, undo-contact, profile-next, profile-previous, sort, density, mark-contact, import-contacts, export-all, export-selected, export-filtered, page-down, page-up, top, bottom.

```toml
[keybindings]
quit = ["x"]
context = ["q"]
new-contact = ["N"]
```

Phase 3 IDs: task-new, activity-log, complete, reopen, snooze, reschedule, priority, stage-next, stage-previous, link, convert-task, followup-schedule, flag, duplicate, dismiss, reviewed, open-external, milestone-add, collection-save, date-add, relationship-add, reminder-add, meeting-log, call-log, note-add, project-new, project-open, item-new, pipeline-new, pipeline-edit, pipeline-archive, pipeline-move, pipeline-up, pipeline-down, stage-add, stage-rename, stage-remove, stage-left, stage-right, relationship-open, relationship-notes, inbox-review, radar-open, radar-settings, collections-open, reminders-open, followups-open, merge-contacts.

Phase 3 default keys are soft: if your existing `[keybindings]` already gives one of them to another command (for example `quit = ["x"]`), your binding wins, the Phase 3 command keeps working from the palette, and you can give it a different key. Two explicit bindings for the same key are still an error.
