# Import/export behavior

Import is preview-first and insert-only. Every imported entity receives a new UUID; intra-batch references are remapped. Existing contacts are never updated by matching an email, name or foreign UID. Matching names/emails/normalized phones are warnings, including duplicates within the incoming file. They are initially unchecked. Space explicitly permits a separate duplicate record.

## Workflow

1. `I`: enter a local file path. Content beginning with BEGIN:VCARD is vCard; other content is parsed as CSV.
2. CSV: review header detection (`h`), choose columns with Up/Down, and change mappings with Left/Right. Enter builds the dry-run preview.
3. Preview: inspect validation errors and duplicates. Space toggles valid rows. `b` returns to CSV mapping. Esc cancels with no changes.
4. Enter opens the final confirmation. `y` imports the selected records; `n` returns to review.
5. Progress reports written records inside the still-uncommitted transaction. A failure rolls back the whole selected batch, including automatically created organizations. The final summary appears only after commit.

The import file is read once for mapping/preview, so changing it externally does not silently change the confirmed batch. Regular files up to 50 MiB are supported. Malformed CSV/vCard structure rejects the file; invalid mapped records are individually reported. Fix the file and reopen it to repair invalid vCards. Name collisions for memberships or relationship targets require unique names or UUIDs. A new membership organization is created atomically; relationship targets must resolve.

## CSV

Header detection and defaults recognize common name, first/last name, email, phone, company, note and category headers, including Google-style email/phone value columns. Users can disable headers for headerless CSV. Repeated mapped email/phone columns produce separate methods. RFC-style quoted values and multiline fields use Go's encoding/csv parser. Unexpected row widths are errors.

Supported mappings: display_name, given, middle, family, prefix, suffix, nickname, email, phone, website, organization, title, department, notes, birthday, anniversary, tags, street, city, region, postal_code, country, kind, tidecontacts_json, or ignore.

CSV exports include readable common columns plus `tidecontacts_json`, a full aggregate extension. On reimport, that extension is authoritative: it preserves all methods, memberships, custom fields, aliases, flags, dates, notes and unknown extension data that a flat CSV cannot express. To remap only the visible columns, explicitly map tidecontacts_json to ignore.

## vCard

The MIT-licensed emersion/go-vcard parser is pinned to the revision already used by TideMail. The inspected parser handles v3/v4 structured names, fields, folded lines, parameters and text escapes. Common structured names, multiple emails/phones, URLs, messaging/social fields, postal components, ORG/TITLE, NOTE, dates and categories map to the domain model. Yearless dates use --MM-DD. Foreign UID and unrecognized properties are retained in the vcard extension; they never become local primary keys.

Exports write v4 and include `X-TIDECONTACTS` for a full aggregate round-trip, analogous to the CSV JSON column. Standard fields are also emitted for other address books. Unknown vCard properties are retained where the parser can represent them. The aggregate extension is authoritative when importing a TideContacts export.

Limitations: not every vendor-specific vCard dialect, legacy charset or quoted-printable encoding is supported. The upstream parser can skip malformed property lines; original raw-file byte preservation is not promised. Standard vCard ORG/TITLE cannot represent every role/date or arbitrary typed field, which is why the full extension is included. Imported photos remain references; the app does not fetch or render them. File imports do not preserve local history or revisions, because they are new local records.

## Export publication

`E` exports the current filtered collection. The command palette also exports all nondeleted people/organizations, including archived records, or marked records (Space), falling back to the current selection. Private notes are included and the TUI confirmation says so.

The destination suffix chooses CSV or vCard. A private temporary file is written and synced in the destination directory, then published with an atomic hard link that fails if the destination already exists. No rename-overwrite race is possible. Filesystems that cannot create hard links report an error; there is no unsafe overwrite fallback. Temporary files are removed after success or failure.
