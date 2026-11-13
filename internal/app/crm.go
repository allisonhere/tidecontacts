package app

import (
	"context"
	"time"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
)

// ActivityFilter selects one bounded page of the timeline, newest first.
type ActivityFilter struct {
	Query               search.Query
	EntityID, ProjectID string
	TaskID, Include     string    // Include forces one activity into the page (link following)
	Before              time.Time // exclusive paging cursor; zero starts at the newest
	Limit               int
}

// TaskFilter selects tasks. View is one of inbox, today, upcoming, waiting, open or empty for all.
type TaskFilter struct {
	View                string
	Query               search.Query
	EntityID, ProjectID string
	Sort                string // due (default), priority, title, updated, created
	Limit               int
}

// CRM is the application-owned persistence boundary for Phase 3 records. Every
// operation works offline against local storage; nothing here reaches the network.
type CRM interface {
	Contacts
	MergeContacts(ctx context.Context, keepID, mergeID string) (domain.Contact, error)
	Link(ctx context.Context, owner, target domain.Ref, on bool) error

	Activities(context.Context, ActivityFilter) ([]domain.Activity, error)
	Activity(context.Context, string) (domain.Activity, error)
	ActivityTypes(context.Context) ([]string, error)
	SaveActivity(context.Context, domain.Activity) (domain.Activity, error)
	UpsertExternalActivity(ctx context.Context, a domain.Activity, emails []string) (domain.Activity, error)
	DeleteActivity(context.Context, string) error

	Tasks(context.Context, TaskFilter) ([]domain.Task, error)
	Task(context.Context, string) (domain.Task, error)
	SaveTask(context.Context, domain.Task) (domain.Task, error)
	CompleteTask(context.Context, string) (domain.Task, error)
	ReopenTask(context.Context, string) (domain.Task, error)
	DuplicateTask(context.Context, string) (domain.Task, error)
	DeleteTask(context.Context, string) error

	Reminders(ctx context.Context, dueOnly bool) ([]domain.Reminder, error)
	SaveReminder(context.Context, domain.Reminder) (domain.Reminder, error)
	SnoozeReminder(ctx context.Context, id string, until time.Time) error
	DismissReminder(context.Context, string) error

	Projects(ctx context.Context, status string, q search.Query) ([]domain.Project, error)
	Project(context.Context, string) (domain.Project, error)
	SaveProject(context.Context, domain.Project) (domain.Project, error)
	DeleteProject(context.Context, string) error

	Pipelines(ctx context.Context, archived bool) ([]domain.Pipeline, error)
	SavePipeline(context.Context, domain.Pipeline) (domain.Pipeline, error)
	MovePipeline(ctx context.Context, id string, delta int) error
	Items(ctx context.Context, pipelineID string, q search.Query) ([]domain.PipelineItem, error)
	Item(context.Context, string) (domain.PipelineItem, error)
	SaveItem(context.Context, domain.PipelineItem) (domain.PipelineItem, error)
	MoveItem(ctx context.Context, id, stageID string) error
	DeleteItem(context.Context, string) error

	FollowUp(context.Context, string) (domain.FollowUp, error)
	FollowUps(context.Context) ([]domain.FollowUp, error)
	SetFollowUp(context.Context, domain.FollowUp) error
	Dates(context.Context, string) ([]domain.ImportantDate, error)
	UpcomingDates(ctx context.Context, days int) ([]domain.ImportantDate, error)
	SaveDate(context.Context, domain.ImportantDate) error
	DeleteDate(context.Context, string) error
	SetFlag(ctx context.Context, entityID string, on bool, note string) error
	SetRelationshipNotes(ctx context.Context, entityID, notes string) error
	Connections(context.Context, string) (domain.Connections, error)
	Dossier(context.Context, string) (domain.Dossier, error)

	Inbox(context.Context) ([]domain.InboxItem, error)
	InboxAct(ctx context.Context, key, action, until string) error
	Radar(context.Context, domain.RadarConfig) ([]domain.Signal, error)
	RadarConfig(context.Context) (domain.RadarConfig, error)
	SaveRadarConfig(context.Context, domain.RadarConfig) error

	Collections(context.Context) ([]domain.Collection, error)
	SaveCollection(context.Context, domain.Collection) (domain.Collection, error)
	DeleteCollection(context.Context, string) error
}
