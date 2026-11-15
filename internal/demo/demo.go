// Package demo builds a disposable fictional profile, never the live address book.
package demo

import (
	"context"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"os"
	"path/filepath"
	"time"
)

func Paths(live config.Paths) config.Paths {
	root := filepath.Join(filepath.Dir(live.Database), "demo")
	return config.Paths{Config: filepath.Join(root, "config.toml"), Database: filepath.Join(root, "tidecontacts.db"), Log: filepath.Join(root, "demo.log")}
}

// Reset removes only the fixed demo subtree derived from the live profile.
func Reset(live config.Paths) error { return os.RemoveAll(filepath.Dir(Paths(live).Database)) }

func Seed(ctx context.Context, s *sqlite.Store) error {
	done, err := s.Get(ctx, "demo-seeded")
	if err != nil || done != "" {
		return err
	}
	now := time.Now()
	people := []domain.Contact{}
	for i, name := range []string{"Alex Rivera", "Morgan Chen", "Sam Taylor", "Jamie Patel", "Robin Ellis", "Casey Brooks"} {
		c := domain.NewContact("person")
		c.Nickname = name
		c.Methods = []domain.Method{{Kind: "email", Value: fmt.Sprintf("person%d@example.com", i+1)}}
		c.Tags = []string{"demo"}
		c.Notes = "Fictional demo contact. Safe to edit."
		people = append(people, c)
	}
	org := domain.NewContact("organization")
	org.DisplayName = "Harbor Studio"
	people[0].Relationships = []domain.Relationship{{TargetID: people[1].ID, Type: "colleague"}}
	people[0].Memberships = []domain.Membership{{Organization: "Harbor Studio", Title: "Designer", Current: true}}
	people = append(people, org)
	people, err = s.SaveContacts(ctx, people, "demo")
	if err != nil {
		return err
	}
	for i := 0; i < 6; i++ {
		refs := []domain.Ref{{ID: people[i].ID}}
		_, err = s.SaveActivity(ctx, domain.Activity{Type: "meeting", Title: "Planning with " + people[i].DisplayName, OccurredAt: now.AddDate(0, 0, -i-1), Entities: refs})
		if err != nil {
			return err
		}
		_, err = s.SaveTask(ctx, domain.Task{Title: "Follow up with " + people[i].DisplayName, Due: now.AddDate(0, 0, i-2).Format("2006-01-02"), Priority: i % 4, Entities: refs})
		if err != nil {
			return err
		}
	}
	_, err = s.SaveProject(ctx, domain.Project{Name: "Harbor launch", Members: []domain.Ref{{ID: people[0].ID}}, Target: now.AddDate(0, 0, 14).Format("2006-01-02")})
	if err != nil {
		return err
	}
	p, err := s.SavePipeline(ctx, domain.Pipeline{Name: "Demo opportunities", Stages: []domain.Stage{{Name: "New"}, {Name: "In progress"}, {Name: "Done"}}})
	if err != nil {
		return err
	}
	_, err = s.SaveItem(ctx, domain.PipelineItem{Title: "Harbor workshop", PipelineID: p.ID, StageID: p.Stages[0].ID, Entities: []domain.Ref{{ID: people[1].ID}}})
	if err != nil {
		return err
	}
	_, err = s.SaveReminder(ctx, domain.Reminder{Title: "Review demo follow-ups", At: now.Add(time.Hour), Entity: domain.Ref{ID: people[0].ID}})
	if err != nil {
		return err
	}
	if err = s.SetFollowUp(ctx, domain.FollowUp{Entity: domain.Ref{ID: people[2].ID}, NextDate: now.AddDate(0, 0, -1).Format("2006-01-02")}); err != nil {
		return err
	}
	return s.Put(ctx, "demo-seeded", "1")
}
