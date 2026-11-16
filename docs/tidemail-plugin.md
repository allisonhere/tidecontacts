# Simple contacts in TideMail

The TideContacts plugin provides a simple address book inside TideMail. The
standalone TideContacts app remains the advanced CRM. Both views use the same
TideContacts database; there is no second copy of a contact to synchronize.

## Build and install

This development feature requires the updated TideMail source in `../tidemail`.
Older TideMail releases do not recognize `contacts_write` and cannot show this UI.

```sh
make install-tidemail-plugin
(cd ../tidemail && go build -buildvcs=false -o ../tidecontact/bin/tidemail .)
./bin/tidemail
```

Installation adds the plugin to TideMail's normal plugin directory. It does not
read, migrate or import contacts. Starting the new contacts view opens the normal
TideContacts database, applying its existing migrations if needed. Keep a backup
before opening older databases with a new version.

With the plugin installed, press **C**. If another contacts provider is explicitly
configured, it is preserved; select `provider = "tidecontacts"` under `[contacts]`
in TideMail's config to use this bridge instead. **L** in the simple view opens
TideMail's original local address book, whose contents remain separate.

## Everyday actions

- **n**: add a person (email is required).
- **e**: edit name, email, phone, organization and a note. Other CRM fields and
  additional contact methods are preserved. Concurrent edits are rejected.
- **/**: search, then Enter to load the results; Esc clears the query.
- **c** or **Enter**: compose an email to the selected person.
- **A**: open Advanced CRM. The installed plugin bundles its matching TideContacts executable.
- **L**: TideMail's original local contacts manager.
- **Esc**: close the view.

The list loads at most 200 people; search to narrow larger address books. Only
active people with an email address appear. Duplicate email addresses are rejected
when saving. Recent correspondence shows up to 15 matching subjects and dates from
TideMail's local cache, checked by exact participant email addresses. Message bodies
and attachments never cross the plugin bridge. Recipient Ctrl+T lookup also uses
the plugin when no other provider is configured.

No external address-book sync, automatic native-contact import, or contact deletion
ships in this version. Existing TideMail local contacts remain available with L.

## Try it without live data

```sh
make tidemail-demo-build
python3 scripts/tidemail_demo.py
```

Press **C** for fictional demo people and recent correspondence. This launcher
keeps TideMail configuration, mail and contacts under `bin/tidemail-demo`, and
uses TideContacts' separate demo database. Advanced CRM stays in demo mode.
No real accounts or address books are loaded. Your changes persist in this demo
profile. The plugin is installed into this demo only; live integration remains
an explicit `make install-tidemail-plugin` action.

`python3 scripts/tidemail_plugin_smoke.py` exercises discovery, add/save, search
and compose in a temporary profile and checks that the normal contacts database
was not opened.

## Which address book am I editing?

The shared view says **Address book: TideContacts**, with a reminder that it is
shared with standalone TideContacts. Demo mode says **TideContacts DEMO** and
explicitly identifies real contacts as separate. These labels remain visible
while adding or editing. The original manager says **TideMail local** and
**Stored only in TideMail; separate from TideContacts**. No contacts are moved
automatically between those books.
