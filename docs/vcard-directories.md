# Connected address books

TideContacts can connect to local address-book folders maintained by vdirsyncer, khard, or another tool, and directly to a Google account over CardDAV. For a local folder, your existing tool continues to handle CardDAV, credentials and remote synchronization; TideContacts reads the local files and never changes them. For Google, TideContacts itself performs the CardDAV sync — read-only, over HTTPS, authenticated with an app password.

## Connect a folder

1. Open **Import data…** (`I`) and choose **khard / vdirsyncer**.
2. Press `n` to browse. Open your address-book folder and press `s` to select the current folder. Press `.` in the browser to show hidden folders such as `~/.contacts` or `~/.config`. Alternatively, press `p` in the folder manager and enter a path (`~/` works).
3. Review the number of cards, press Enter, then `y` to connect.

Connect each address-book folder separately. The reader accepts `.vcf` and `.vcard` files in that folder, including hidden filenames and files containing several cards. It does not search subfolders. Contacts without email addresses remain available in TideContacts; only contacts with email appear in TideMail’s People view or recipient lookup.

## Connect a Google account

1. Create an app password at [myaccount.google.com/apppasswords](https://myaccount.google.com/apppasswords). This requires 2-Step Verification on the Google account; without it, Google has no basic-auth path for CardDAV and the connection will be rejected.
2. Open **Import data…** (`I`), choose **Google Contacts**, and press `n`.
3. Enter the account email and the 16-character app password (Tab switches fields), then press Enter. TideContacts validates the credentials by fetching the full contact list before anything is shown.
4. Review the number of cards, press Enter, then `y` to connect.

The app password is stored in the OS keychain (`secret-tool` on Linux, Keychain Access on macOS), never in the SQLite database or a config file. If no system keychain is available, connecting still works but the password cannot be saved, and the next refresh will fail until you reconnect. Disconnecting (`x`, then `y`) hides the cached contacts but keeps the saved password, so reconnecting from the books list (`Enter` on a disconnected entry) does not require re-entering it.

## Refresh and use in TideMail

Connected folders and Google accounts refresh when TideContacts starts. Use **Refresh connected address books** in the command palette or **Refresh address books** in the menu, or press `r` in the connected-folder modal.

With the TideContacts plugin installed, TideMail refreshes the folders when it loads its TideContacts People view, when you press `r` there, and when you request external recipient lookup with `Ctrl+T`. **TideContacts does not need to be running.** All email addresses on a card are available through Ctrl+T. Normal typing continues to use TideMail’s existing local suggestions.

## Ownership and failures

- Folder and Google contacts are read-only in both apps and through the scripting API. Edit the original vCards or the Google contact, then refresh. TideContacts never writes a contact back to a folder or to Google.
- Local contacts stay separate, even when their names or email addresses match external contacts.
- vCard UID identifies a contact, so renaming its file (or the UID Google assigns a contact) preserves its TideContacts identity and CRM links. Without a UID, a local file's identity uses filename and card position; renaming or reordering those cards creates new identities.
- A successful complete refresh updates changed cards and hides cards removed from that folder or account. Their history and CRM references remain. If a card returns with the same UID, the original contact is restored.
- A missing/unreadable folder, an unreachable Google account or rejected credentials, a malformed file, a duplicate UID, or an invalid contact rejects that book's refresh and retains the previous snapshot. Other connected books can still refresh. TideMail displays a warning while continuing to show cached contacts.
- A website value that isn't a valid URL (common from Google, which stores website types like FTP without a matching `ftp://` requirement, or a bare domain with no scheme) is repaired by adding `https://` where that's enough, or otherwise dropped from that one contact. It never rejects the card or the refresh; everything else on that contact is kept.
- An accessible empty folder, or a Google account with no contacts, is a valid empty address book and hides its cached contacts. A missing folder, or a Google account whose credentials are rejected, never counts as an empty one.
- In the books modal, select a connection and press `x`, then `y` to disconnect. This stops refreshing and hides its cached contacts. Reconnecting a local folder's canonical path, or a Google account's saved app password, restores the same identities. Original files, the Google account, and local contacts remain unchanged.

Folder and Google reads are limited to 10,000 entries/cards and 50 MiB per snapshot. Symlinked vCard files are rejected. Connecting and refreshing live folders or Google accounts is disabled in the demo profile. The schema adds directory ownership tables in migration 4; recipient completion stays read-only and may ask you to open the updated app once to apply that migration. A Google connection needs no schema change: it is stored as an ordinary directory row whose path is `google://<email>`.
