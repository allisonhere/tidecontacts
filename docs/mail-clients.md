# aerc and NeoMutt address-book compatibility

TideContacts can provide recipient candidates to both clients. It uses the same
address book as the standalone app and TideMail plugin; none needs to be running.
Both clients can also save the sender of a selected message.

## Save a sender

Merge the [aerc bindings](../contrib/mail-clients/binds.conf) into the matching
sections of `binds.conf`, or source the [NeoMutt fragment](../contrib/mail-clients/neomuttrc).
With a single message selected or open, press **,cs** (comma, c, s).
The command prints **Saved** for a new person or **Already saved** for an existing
address. It compares email addresses case-insensitively, including secondary
addresses, archived contacts, and cached directory contacts. Existing records keep
their name and fields.

The binding pipes the message into `tidecontacts contacts capture-sender`.
Only the single From mailbox supplies contact data. A missing or ambiguous From
header produces an error. For a standalone check, run
`tidecontacts --demo contacts capture-sender < message.eml`. Add `--json` for
the contact ID and a boolean indicating whether the command created a record.

## Try the output first

```sh
tidecontacts --demo contacts query --format aerc --query Alex
tidecontacts --demo contacts query --format neomutt --query Alex
tidecontacts contacts query --query "O'Connor"  # JSON is still the default
```

An empty query browses available recipients. Queries are literal, case-insensitive
word matches against name, email and organization, with every word required. Shell
syntax, regular expressions and CRM filter operators have no special meaning.
Private notes are never searched or returned. People and organizations are both
included by default; use `--kind person` or `--kind organization` to narrow that.

Only active, non-deleted contacts with valid email methods appear. All email
methods are eligible, with preferred methods first within a person. Duplicate
email addresses are collapsed without regard to case. Results have deterministic
ordering. Tabs, newlines and terminal controls cannot create extra output fields
or rows. No phone numbers, notes, biographies or other CRM fields are exposed.

Normal lookup opens SQLite read-only and does not create, seed or migrate a real
database. Open TideContacts or run `tidecontacts --check` once if the database is
not initialized. Explicit `--demo` seeds only its separate demo profile when
needed; `--database /absolute/path` selects another existing book. The CLI profile
is explicit and does not inherit the TideMail plugin's saved real/demo choice.

## NeoMutt: searched recipient completion

Add this to `neomuttrc`, or source the supplied
[configuration fragment](../contrib/mail-clients/neomuttrc):

```muttrc
set query_command = "tidecontacts contacts query --format neomutt -- %s"
```

Type part of a name or address in a recipient field and press **Ctrl+T**. **Q** in
the index opens NeoMutt's query browser. To replace Tab's normal alias completion:

```muttrc
bind editor <Tab> complete-query
```

NeoMutt adds shell quoting around `%s` itself. Do not add extra quotes. The `--`
keeps a query starting with `-` from becoming a TideContacts option. For demo-only
testing, add `--demo` before the final `--`.

The NeoMutt format begins with a single status line, then tab-separated
`email`, `name`, `organization` fields. No matches returns exit 1 and a status line;
runtime failures also provide a one-line status plus a JSON diagnostic on stderr.
Successful results exit 0. Default limit is 200; `--limit` accepts 1–5,000.

References: [external queries](https://neomutt.org/guide/advancedusage#query),
[query_command quoting](https://neomutt.org/guide/reference#query-command).

## aerc: safe recipient list completion

Merge this into the `[compose]` section of `aerc.conf`, or use the supplied
[configuration fragment](../contrib/mail-clients/aerc.conf):

```ini
[compose]
address-book-cmd = tidecontacts contacts query --format aerc --limit 99
```

Press **Tab** in a recipient header to choose from the returned address list. This
safe configuration deliberately returns candidates independently of the typed
prefix. It is not searched completion. A fixed filter can be set per account, for
example `--query example.com`, without interpolating recipient text. The adapter
itself supports filtered queries when invoked directly with `--query TEXT`.

The inspected upstream aerc completion code replaces `%s` with raw recipient
text before running `sh -c`. Adding your own single or double quotes does not
reliably handle arbitrary apostrophes and shell syntax. This integration therefore
does not recommend a `%s` configuration. Fully searched in-client completion needs
aerc to quote that substitution or pass the query separately; the database and
adapter do not need to change. No client source patch is installed here.

The aerc format is just tab-separated `email` and `name`, without a header. Empty
results produce no output and exit 0. aerc's current reader accepts at most 100
lines and reports that limit as an overflow; this adapter defaults to 99 for aerc
and rejects larger explicit limits. Use a fixed filter to narrow larger books.
`edit-headers=true` disables aerc's address-book completion; editor completion is
then configured separately. For demo testing, add `--demo` to the command.

References: [aerc compose configuration](https://raw.githubusercontent.com/rjarry/aerc/master/doc/aerc-config.5.scd),
[upstream completion implementation](https://github.com/rjarry/aerc/blob/master/completer/completer.go).

## JSON and limits

Without `--format`, `contacts query` returns the normal versioned JSON envelope
with recipient objects containing `contact_id`, `email`, `name`, `organization`
and `preferred`. JSON/NeoMutt default to 200 results and accept `--limit 1..5000`.
JSON responses report their limit and `possibly_truncated`. The raw formats have
no pagination markers. No-match exit behavior intentionally differs between aerc
and NeoMutt; other CLI commands retain their JSON-only stdout behavior.

These fragments are examples; they do not overwrite your mail-client
configuration or configure accounts. Neither client is installed in this workspace,
so verification covers their documented wire formats, subprocess invocation,
read-only access and shell quoting simulations, rather than an interactive client
session.

Connected vCard-folder contacts are included in the cached address book. Recipient queries do not scan or refresh those folders; refresh through TideContacts or TideMail first when needed. To bring existing Mutt aliases into TideContacts, use [Import data… → Mutt / NeoMutt aliases](import-export.md#mutt--neomutt-aliases). [Connect vdirsyncer folders](vcard-directories.md) for repeatable updates.
