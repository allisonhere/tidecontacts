package domain

import "strings"

const DirectorySourcePrefix = "vcard-directory:"

func (c Contact) DirectoryReadOnly() bool { return strings.HasPrefix(c.Source, DirectorySourcePrefix) }

type ContactDirectory struct {
	ID, Path, LastRefreshed string
	Revision, Contacts      int
}
type DirectoryCard struct {
	Key, Fingerprint string
	Contact          Contact
}
type DirectorySnapshot struct {
	Path  string
	Cards []DirectoryCard
}
type DirectoryReport struct {
	Path                               string
	Added, Updated, Removed, Unchanged int
}
