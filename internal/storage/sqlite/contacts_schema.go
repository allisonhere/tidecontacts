package sqlite

const contactsSchema = `
CREATE TABLE entities (
 id TEXT PRIMARY KEY, kind TEXT NOT NULL CHECK(kind IN ('person','organization')),
 display_name TEXT NOT NULL, notes TEXT NOT NULL DEFAULT '', biography TEXT NOT NULL DEFAULT '',
 avatar TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT 'manual', favorite INTEGER NOT NULL DEFAULT 0,
 archived INTEGER NOT NULL DEFAULT 0, deleted INTEGER NOT NULL DEFAULT 0,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL, revision INTEGER NOT NULL,
 extensions TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE people (id TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
 given_name TEXT NOT NULL, middle_name TEXT NOT NULL, family_name TEXT NOT NULL,
 prefix TEXT NOT NULL, suffix TEXT NOT NULL, nickname TEXT NOT NULL, phonetic TEXT NOT NULL,
 birthday TEXT NOT NULL, anniversary TEXT NOT NULL);
CREATE TABLE organizations (id TEXT PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE, website TEXT NOT NULL, industry TEXT NOT NULL);
CREATE TABLE aliases (entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, value TEXT NOT NULL, PRIMARY KEY(entity_id,value));
CREATE TABLE methods (id TEXT PRIMARY KEY, entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
 kind TEXT NOT NULL, label TEXT NOT NULL, value TEXT NOT NULL, normalized TEXT NOT NULL, preferred INTEGER NOT NULL,
 notes TEXT NOT NULL, source TEXT NOT NULL, verification TEXT NOT NULL,
 po_box TEXT NOT NULL, extended TEXT NOT NULL, street TEXT NOT NULL, city TEXT NOT NULL, region TEXT NOT NULL, postal_code TEXT NOT NULL, country TEXT NOT NULL);
CREATE UNIQUE INDEX one_preferred_method ON methods(entity_id,kind) WHERE preferred=1;
CREATE INDEX method_match ON methods(normalized);
CREATE INDEX method_owner ON methods(entity_id);
CREATE TABLE memberships (id TEXT PRIMARY KEY, person_id TEXT NOT NULL REFERENCES people(id) ON DELETE CASCADE,
 organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT, title TEXT NOT NULL, department TEXT NOT NULL,
 start_date TEXT NOT NULL, end_date TEXT NOT NULL, current INTEGER NOT NULL);
CREATE INDEX membership_org ON memberships(organization_id);
CREATE INDEX membership_person ON memberships(person_id);
CREATE TABLE relationship_types (name TEXT PRIMARY KEY);
INSERT INTO relationship_types VALUES ('colleague'),('friend'),('family'),('collaborator'),('client');
CREATE TABLE relationships (id TEXT PRIMARY KEY, source_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
 target_id TEXT NOT NULL REFERENCES entities(id) ON DELETE RESTRICT, type TEXT NOT NULL REFERENCES relationship_types(name), notes TEXT NOT NULL,
 CHECK(source_id != target_id));
CREATE INDEX relationship_source ON relationships(source_id);
CREATE INDEX relationship_target ON relationships(target_id);
CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE);
CREATE TABLE entity_tags (entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, tag_id INTEGER NOT NULL REFERENCES tags(id), PRIMARY KEY(entity_id,tag_id));
CREATE TABLE field_definitions (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE COLLATE NOCASE, type TEXT NOT NULL, options TEXT NOT NULL);
CREATE TABLE field_values (entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, field_id INTEGER NOT NULL REFERENCES field_definitions(id), value TEXT NOT NULL, PRIMARY KEY(entity_id,field_id));
CREATE TABLE history (entity_id TEXT NOT NULL REFERENCES entities(id) ON DELETE CASCADE, revision INTEGER NOT NULL, at TEXT NOT NULL, action TEXT NOT NULL, snapshot TEXT NOT NULL, PRIMARY KEY(entity_id,revision));
CREATE INDEX entity_name ON entities(kind,display_name COLLATE NOCASE,id);
CREATE INDEX entity_updated ON entities(updated_at);
CREATE VIRTUAL TABLE contact_search USING fts5(entity_id UNINDEXED, text, tokenize='unicode61 remove_diacritics 2');
`
