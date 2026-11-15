package domain

import "time"

// ArchiveFormat identifies a TideContacts CRM export; ArchiveVersion changes only
// when the shape changes incompatibly.
const (
	ArchiveFormat  = "tidecontacts-crm"
	ArchiveVersion = 1
)

// Archive is the portable, versioned form of every CRM record that is not a contact.
// Contacts travel in the existing CSV/vCard export; archive links refer to them by ID
// and are dropped (with a warning) on import when the contact does not exist locally.
type Archive struct {
	Format     string          `json:"format"`
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Activities []Activity      `json:"activities"`
	Tasks      []Task          `json:"tasks"`
	Reminders  []Reminder      `json:"reminders"`
	Projects   []Project       `json:"projects"`
	Pipelines  []ArchivePipe   `json:"pipelines"`
	Follow     []FollowUp      `json:"follow_ups"`
	Dates      []ImportantDate `json:"important_dates"`
	Meta       []ArchiveMeta   `json:"relationship_meta"`
	Collection []Collection    `json:"collections"`
}

// ArchivePipe is a pipeline with its items and their stage history.
type ArchivePipe struct {
	Pipeline
	Items []PipelineItem `json:"items"`
}

type ArchiveMeta struct {
	Entity   Ref    `json:"entity"`
	Notes    string `json:"notes"`
	Flagged  bool   `json:"flagged"`
	FlagNote string `json:"flag_note"`
}

// ImportReport summarizes an import. Existing IDs are never overwritten.
type ImportReport struct {
	Added, Skipped map[string]int
	Warnings       []string
}

func NewImportReport() ImportReport {
	return ImportReport{Added: map[string]int{}, Skipped: map[string]int{}}
}
