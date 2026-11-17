# Connected vCard address books

TideContacts can connect to local address-book folders maintained by vdirsyncer, khard, or another tool. Your existing tool continues to handle CardDAV, credentials and remote synchronization. TideContacts reads the local files and never changes them.

## Connect a folder

1. Open **Import data…** (`I`) and choose **khard / vdirsyncer**.
2. Press `n` to browse. Open your address-book folder and press `s` to select the current folder. Press `.` in the browser to show hidden folders such as `~/.contacts` or `~/.config`. Alternatively, press `p` in the folder manager and enter a path (`~/` works).
3. Review the number of cards, press Enter, then `y` to connect.

Connect each address-book folder separately. The reader accepts `.vcf` and `.vcard` files in that folder, including hidden filenames and files containing several cards. It does not search subfolders. Contacts without email addresses remain available in TideContacts; only contacts with email appear in TideMail’s People view or recipient lookup.

## Refresh and use in TideMail

Connected folders refresh when TideContacts starts. Use **Refresh connected address books** in the command palette or **Refresh address books** in the menu, or press `r` in the connected-folder modal.

With the TideContacts plugin installed, TideMail refreshes the folders when it loads its TideContacts People view, when you press `r` there, and when you request external recipient lookup with `Ctrl+T`. **TideContacts does not need to be running.** All email addresses on a card are available through Ctrl+T. Normal typing continues to use TideMail’s existing local suggestions.

## Ownership and failures

- Folder contacts are read-only in both apps and through the scripting API. Edit their original vCards with your existing tool, then refresh. This first version does not write contacts back to the folder or server.
- Local contacts stay separate, even when their names or email addresses match external contacts.
- vCard UID identifies a contact, so renaming its file preserves its TideContacts identity and CRM links. Without a UID, identity uses filename and card position; renaming or reordering these cards creates new identities.
- A successful complete refresh updates changed cards and hides cards removed from that folder. Their history and CRM references remain. If a card returns with the same UID, the original contact is restored.
- A missing or unreadable folder, malformed file, duplicate UID or invalid contact rejects that folder’s refresh and retains the previous snapshot. Other connected folders can still refresh. TideMail displays a warning while continuing to show cached contacts.
- An accessible empty folder is a valid empty address book and hides its cached contacts. A missing folder never counts as an empty one.
- In the folder modal, select a connection and press `x`, then `y` to disconnect. This stops refreshing and hides its cached contacts. Reconnecting the same canonical path restores the same identities. Original files and local contacts remain unchanged.

Folder reads are limited to 10,000 entries/cards and 50 MiB per snapshot. Symlinked vCard files are rejected. Connecting and refreshing live folders is disabled in the demo profile. The schema adds directory ownership tables in migration 4; recipient completion stays read-only and may ask you to open the updated app once to apply that migration.
