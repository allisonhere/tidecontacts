# Simple contacts in TideMail

The TideContacts plugin provides a simple address book inside TideMail. The
standalone TideContacts app remains the advanced CRM. Both views use the same
TideContacts database; there is no second copy of a contact to synchronize.

## Build and install

This development feature requires the updated TideMail source in `../tidemail`.
Older TideMail releases do not recognize `contacts_write` and cannot show this UI. The checkout used by `go run .` must include the TideContacts integration; an updated binary elsewhere does not change that checkout.

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

### Nix

The Nix package ships the plugin directory, with its matching TideContacts
executable, at `share/tidemail/plugins/tidecontacts`. Instead of
`make install-tidemail-plugin`, link it into TideMail's plugin directory, for
example from a `nix profile` install:

```sh
mkdir -p ~/.config/tidemail/plugins
ln -s ~/.nix-profile/share/tidemail/plugins/tidecontacts \
  ~/.config/tidemail/plugins/tidecontacts
```

With Home Manager, use
`xdg.configFile."tidemail/plugins/tidecontacts".source = "${pkgs.tidecontacts}/share/tidemail/plugins/tidecontacts";`.
The link follows Nix upgrades, so the plugin and Advanced CRM stay in step with
the installed TideContacts.

## Everyday actions

- **n**: add a person (email is required).
- **e**: edit a local person’s name, email, phone, organization and note. Folder contacts are read-only. Other CRM fields and
  additional contact methods are preserved. Concurrent edits are rejected.
- **r**: reload People and refresh connected vCard folders.
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
the plugin when no other provider is configured. All email methods on a contact are available through Ctrl+T; the People list shows its preferred email when set, otherwise its first email.

## Connected address books

Open **A → Import data… → khard / vdirsyncer** to connect a local vCard folder. In the folder manager, press `n` to browse and `s` to select the current folder, or `p` to enter its path. Preview and confirm the connection.

Folder contacts carry a **folder · read-only** label in People. Edit them in the original address-book tool, then press **r**. TideMail refreshes connected folders whenever it loads or refreshes People or handles Ctrl+T lookup. The standalone TideContacts app does not need to run. Successful refreshes update changed cards and hide removed cards; failed scans retain the last good contacts and display a warning. Local contacts with matching names/emails stay separate. See [folder connections](vcard-directories.md) for UID behavior, limits and disconnect/reconnect.

vdirsyncer owns CardDAV synchronization and credentials. This integration reads its local files. TideMail's native contacts are not imported automatically, and the simple People view has no delete action; use **L** for the separate native address book.

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

## Plugin settings

In TideMail, open **Settings → Plugins**, select **TideContacts**, and press
**Enter** or **s**. Changes save immediately.

- **Default contacts view:** choose what C opens: TideContacts or TideMail local.
  An explicitly configured different contacts provider remains respected.
- **Contact data:** choose real or demo TideContacts data. This setting applies
  only to TideContacts, not TideMail's separate local contacts. Demo data is seeded
  once and stays separate from the real database. Normal TideMail hides live
  correspondence while viewing demo contacts. The isolated demo launcher always
  forces demo data, including for Advanced CRM.
- **Check connection:** shows the profile and active people count. This is a
  read-only check; it does not create or migrate the database. A missing database
  shows Ready and is created only when you open that address book.
- **Open Advanced CRM:** launches the bundled app with the selected profile.

TideContacts settings are local preferences. Connect and manage vCard folders in Advanced CRM’s import modal; they are stored in the selected TideContacts database rather than these plugin settings.

Advanced CRM runs interactively in TideMail’s terminal while TideMail pauses.
Quit the CRM to return to TideMail. The child inherits the terminal directly,
so the renderer’s output serialization does not turn its output into a pipe.
The terminal smoke test verifies launch and return as well as basic contacts.

## A red plugin or missing settings

If TideMail reports **manifest has unknown keys: permissions.contacts_write**, its running source/binary lacks the integration. Update that checkout or executable, then restart TideMail. Do not remove the permission from the manifest: adding and editing local contacts requires it.

When starting with `go run .`, run it inside the compatible TideMail checkout. Reinstalling the plugin or updating `~/.local/bin/tidemail` alone does not update that source tree. **Check connection** is read-only; opening People can apply the matching TideContacts schema migrations.
