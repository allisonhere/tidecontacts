package sqlite

// crmSchema is migration 3 (Phase 3). It only adds tables; Phase 2 rows are untouched.
//
// Conventions: UUID text keys; instants are UTC RFC 3339 text; calendar dates are
// YYYY-MM-DD text in the user's local calendar; junction tables are named
// <owner>_<targets> with <owner>_id and <target>_id columns and cascade from both sides.
// Contacts are tombstoned rather than hard-deleted, so links to them never dangle.
const crmSchema = `
CREATE TABLE activity_types (name TEXT PRIMARY KEY, interaction INTEGER NOT NULL DEFAULT 1);
INSERT INTO activity_types VALUES ('email',1),('call',1),('meeting',1),('conversation',1),('message',1),('event',1),
 ('note',0),('task-completion',0),('project-update',0);
CREATE TABLE activities (
 id TEXT PRIMARY KEY, type TEXT NOT NULL REFERENCES activity_types(name), title TEXT NOT NULL, body TEXT NOT NULL DEFAULT '',
 occurred_at TEXT NOT NULL, timezone TEXT NOT NULL DEFAULT '',
 provenance TEXT NOT NULL DEFAULT 'local' CHECK(provenance IN ('local','imported','external')),
 source_app TEXT NOT NULL DEFAULT '', external_ref TEXT NOT NULL DEFAULT '', external_url TEXT NOT NULL DEFAULT '',
 visibility TEXT NOT NULL DEFAULT 'private' CHECK(visibility IN ('private','shared')),
 deleted INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, revision INTEGER NOT NULL);
CREATE UNIQUE INDEX activity_external ON activities(source_app, external_ref) WHERE external_ref != '';
CREATE INDEX activity_time ON activities(occurred_at, id);
CREATE INDEX activity_type_time ON activities(type, occurred_at);

CREATE TABLE recurrence_rules (
 id TEXT PRIMARY KEY, freq TEXT NOT NULL CHECK(freq IN ('daily','weekly','monthly','yearly')),
 every INTEGER NOT NULL DEFAULT 1 CHECK(every >= 1), by_day TEXT NOT NULL DEFAULT '', month_day INTEGER NOT NULL DEFAULT 0,
 anchor TEXT NOT NULL DEFAULT '', until TEXT NOT NULL DEFAULT '', count INTEGER NOT NULL DEFAULT 0,
 after_completion INTEGER NOT NULL DEFAULT 0);

CREATE TABLE tasks (
 id TEXT PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('inbox','next','scheduled','waiting','completed','canceled')),
 priority INTEGER NOT NULL DEFAULT 0 CHECK(priority BETWEEN 0 AND 3),
 due_date TEXT NOT NULL DEFAULT '', due_time TEXT NOT NULL DEFAULT '', start_date TEXT NOT NULL DEFAULT '',
 timezone TEXT NOT NULL DEFAULT '', snoozed_until TEXT NOT NULL DEFAULT '', completed_at TEXT NOT NULL DEFAULT '',
 series_id TEXT REFERENCES recurrence_rules(id), occurrence INTEGER NOT NULL DEFAULT 0,
 archived INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, revision INTEGER NOT NULL);
CREATE UNIQUE INDEX task_series ON tasks(series_id, occurrence) WHERE series_id IS NOT NULL;
CREATE INDEX task_status_due ON tasks(status, due_date);
CREATE INDEX task_due ON tasks(due_date) WHERE due_date != '';

CREATE TABLE projects (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('planned','active','on-hold','completed','archived')),
 priority INTEGER NOT NULL DEFAULT 0 CHECK(priority BETWEEN 0 AND 3),
 start_date TEXT NOT NULL DEFAULT '', target_date TEXT NOT NULL DEFAULT '', deleted INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, revision INTEGER NOT NULL);
CREATE INDEX project_status ON projects(status, name COLLATE NOCASE);
CREATE TABLE milestones (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE, title TEXT NOT NULL,
 due_date TEXT NOT NULL DEFAULT '', done_at TEXT NOT NULL DEFAULT '', position INTEGER NOT NULL DEFAULT 0);
CREATE INDEX milestone_project ON milestones(project_id, position);

CREATE TABLE activity_entities (activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
 entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(activity_id, entity_id));
CREATE INDEX activity_entities_rev ON activity_entities(entity_id, activity_id);
CREATE TABLE activity_projects (activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
 project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(activity_id, project_id));
CREATE INDEX activity_projects_rev ON activity_projects(project_id, activity_id);
CREATE TABLE activity_tasks (activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
 task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(activity_id, task_id));
CREATE INDEX activity_tasks_rev ON activity_tasks(task_id, activity_id);
CREATE TABLE task_entities (task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(task_id, entity_id));
CREATE INDEX task_entities_rev ON task_entities(entity_id, task_id);
CREATE TABLE task_projects (task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
 project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(task_id, project_id));
CREATE INDEX task_projects_rev ON task_projects(project_id, task_id);
CREATE TABLE project_entities (project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(project_id, entity_id));
CREATE INDEX project_entities_rev ON project_entities(entity_id, project_id);

CREATE TABLE reminders (
 id TEXT PRIMARY KEY, title TEXT NOT NULL, note TEXT NOT NULL DEFAULT '', remind_at TEXT NOT NULL, timezone TEXT NOT NULL DEFAULT '',
 recurrence_id TEXT REFERENCES recurrence_rules(id),
 task_id TEXT REFERENCES tasks(id) ON DELETE CASCADE, entity_id TEXT REFERENCES entities(id) ON DELETE CASCADE,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','dismissed')), snoozed_until TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE INDEX reminder_due ON reminders(status, remind_at);
CREATE INDEX reminder_task ON reminders(task_id);
CREATE INDEX reminder_entity ON reminders(entity_id);

CREATE TABLE pipelines (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
 archived INTEGER NOT NULL DEFAULT 0, position INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE pipeline_stages (id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
 name TEXT NOT NULL, position INTEGER NOT NULL DEFAULT 0);
CREATE INDEX stage_pipeline ON pipeline_stages(pipeline_id, position);
CREATE TABLE pipeline_items (
 id TEXT PRIMARY KEY, pipeline_id TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
 stage_id TEXT NOT NULL REFERENCES pipeline_stages(id) ON DELETE RESTRICT,
 title TEXT NOT NULL, notes TEXT NOT NULL DEFAULT '', next_action TEXT NOT NULL DEFAULT '', stage_entered_at TEXT NOT NULL,
 archived INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, revision INTEGER NOT NULL);
CREATE INDEX item_stage ON pipeline_items(pipeline_id, stage_id, stage_entered_at);
CREATE TABLE pipeline_stage_history (id INTEGER PRIMARY KEY, item_id TEXT NOT NULL REFERENCES pipeline_items(id) ON DELETE CASCADE,
 from_stage TEXT NOT NULL DEFAULT '', to_stage TEXT NOT NULL, at TEXT NOT NULL);
CREATE INDEX stage_history_item ON pipeline_stage_history(item_id, id);
CREATE TABLE item_entities (item_id TEXT NOT NULL REFERENCES pipeline_items(id) ON DELETE CASCADE,
 entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(item_id, entity_id));
CREATE INDEX item_entities_rev ON item_entities(entity_id, item_id);
CREATE TABLE item_projects (item_id TEXT NOT NULL REFERENCES pipeline_items(id) ON DELETE CASCADE,
 project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(item_id, project_id));
CREATE INDEX item_projects_rev ON item_projects(project_id, item_id);
CREATE TABLE item_tasks (item_id TEXT NOT NULL REFERENCES pipeline_items(id) ON DELETE CASCADE,
 task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(item_id, task_id));
CREATE INDEX item_tasks_rev ON item_tasks(task_id, item_id);
CREATE TABLE item_activities (item_id TEXT NOT NULL REFERENCES pipeline_items(id) ON DELETE CASCADE,
 activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE, role TEXT NOT NULL DEFAULT '', PRIMARY KEY(item_id, activity_id));
CREATE INDEX item_activities_rev ON item_activities(activity_id, item_id);

CREATE TABLE activity_tags (activity_id TEXT NOT NULL REFERENCES activities(id) ON DELETE CASCADE, tag_id INTEGER NOT NULL REFERENCES tags(id), PRIMARY KEY(activity_id, tag_id));
CREATE TABLE task_tags (task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE, tag_id INTEGER NOT NULL REFERENCES tags(id), PRIMARY KEY(task_id, tag_id));
CREATE TABLE project_tags (project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE, tag_id INTEGER NOT NULL REFERENCES tags(id), PRIMARY KEY(project_id, tag_id));
CREATE TABLE item_tags (item_id TEXT NOT NULL REFERENCES pipeline_items(id) ON DELETE CASCADE, tag_id INTEGER NOT NULL REFERENCES tags(id), PRIMARY KEY(item_id, tag_id));

CREATE TABLE relationship_meta (entity_id TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
 notes TEXT NOT NULL DEFAULT '', flagged INTEGER NOT NULL DEFAULT 0, flag_note TEXT NOT NULL DEFAULT '',
 flagged_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL);
CREATE TABLE followup_prefs (entity_id TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
 every INTEGER NOT NULL DEFAULT 0, unit TEXT NOT NULL DEFAULT 'day' CHECK(unit IN ('day','week','month')),
 never INTEGER NOT NULL DEFAULT 0, next_date TEXT NOT NULL DEFAULT '', snoozed_until TEXT NOT NULL DEFAULT '',
 set_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE important_dates (id TEXT PRIMARY KEY, entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
 label TEXT NOT NULL, date TEXT NOT NULL, annual INTEGER NOT NULL DEFAULT 1, notes TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE INDEX important_date_entity ON important_dates(entity_id);

CREATE TABLE inbox_state (key TEXT PRIMARY KEY, kind TEXT NOT NULL, fingerprint TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','reviewed','dismissed','snoozed')),
 snoozed_until TEXT NOT NULL DEFAULT '', first_seen TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE collections (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE, description TEXT NOT NULL DEFAULT '',
 entity TEXT NOT NULL CHECK(entity IN ('person','organization','task','activity','project','item')),
 query TEXT NOT NULL DEFAULT '', sort TEXT NOT NULL DEFAULT '', grouping TEXT NOT NULL DEFAULT '',
 pinned INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);

CREATE VIRTUAL TABLE crm_search USING fts5(kind UNINDEXED, record_id UNINDEXED, text, tokenize='unicode61 remove_diacritics 2');
`
