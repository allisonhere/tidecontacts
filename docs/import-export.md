# Import/export behavior

Open **Import data…** from the menu or press `I`. The import modal offers CSV contacts, vCard address books, TideContacts CRM archives, Mutt/NeoMutt aliases, khard/vdirsyncer folders, and Google Contacts. Active sources stay above dimmed, disabled planned sources. Choose a source with ↑/↓ and Enter, then browse folders and supported files. Enter opens a folder or selects a file; ←/Backspace goes to its parent; `p` opens direct path entry (`~/` supported); `b` returns to sources. `.` toggles hidden files and folders, initially off. Esc closes the modal. Mapping, preview, confirmation and results stay in the modal over your workspace. Nothing is written before confirmation.

One-time contact import is preview-first and insert-only. Connected folders instead maintain their own read-only records; see [folder synchronization](vcard-directories.md). Every imported entity receives a new UUID; intra-batch references are remapped. Existing contacts are never updated by matching an email, name or foreign UID. Matching names/emails/normalized phones are warnings, including duplicates within the incoming file. They are initially unchecked. Space explicitly permits a separate duplicate record.

## Workflow

1. Press `I`, choose CSV, vCard or Mutt/NeoMutt aliases, then browse to a file or press `p` to enter its path. Select the CRM source for the [archive workflow](export-import.md), or khard/vdirsyncer/Google Contacts for a [connected address book](vcard-directories.md).
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

## Mutt / NeoMutt aliases

Choose **Mutt / NeoMutt aliases** in the import modal. Browse to your alias file (often `~/.mutt/aliases` or `~/.config/neomutt/aliases`) or use `p` for a path. This source accepts all filenames, including extensionless files. Press `.` to show hidden files and folders such as `.muttrc` and `.mutt`. You can also select a `.muttrc` containing inline aliases.

The importer reads literal `alias [-group name ...] key address [, address ...]` definitions, quoted names and whole address arguments, backslash line continuations, and trailing comments. Each recipient becomes a separate person: its display name comes from the address, its nickname from the alias key, group names become contact tags, and trailing comments become notes. A recipient without a display name uses the alias key. Existing and incoming duplicates are unchecked in preview; Space explicitly includes them as separate contacts.

Configuration is read as data. Other commands, including `source`, are ignored; referenced files are not opened. Import each alias file separately. Dynamic addresses using variables or backticks, alias-to-alias references, `unalias`, and multiple commands on an alias line are rejected with a line number. A malformed alias stops the file before anything is written. NeoMutt `tags:` text in a trailing comment is retained in notes.

Syntax reference: [NeoMutt alias configuration](https://neomutt.org/guide/configuration).

## Planned sources

The source chooser also lists Microsoft Outlook / 365, Mozilla Thunderbird, Apple Contacts / iCloud, Fastmail, Proton Contacts, Nextcloud Contacts, Android Contacts, and Evolution / KDE Kontact. These entries are greyed out and marked **coming soon**. They cannot open files or start an import; each will be enabled when its dedicated importer is implemented. The existing generic CSV and vCard importers remain available.

## Connected vCard folders and Google Contacts

**khard / vdirsyncer** and **Google Contacts** are enabled. Both connect an address book as a read-only source with repeatable refresh, rather than creating an independent one-time import. khard/vdirsyncer connects a local folder; Google Contacts connects a Google account over CardDAV with an email and an app password. See [connected address books](vcard-directories.md) for setup, refresh, failure recovery and disconnect.
