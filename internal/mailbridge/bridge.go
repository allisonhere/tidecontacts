// Package mailbridge implements the local TideMail contact-provider protocol.
package mailbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

type Contact struct {
	ID           string `json:"id"`
	Revision     int    `json:"revision"`
	Name         string `json:"name"`
	Addr         string `json:"addr"`
	Phone        string `json:"phone"`
	Organization string `json:"organization"`
	Note         string `json:"note"`
}
type Request struct {
	API       int             `json:"api"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	Method    string          `json:"method"`
	Data      json.RawMessage `json:"data"`
}
type response struct {
	API       int    `json:"api"`
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Data      any    `json:"data,omitempty"`
	Error     any    `json:"error,omitempty"`
}

func project(c domain.Contact) Contact {
	out := Contact{ID: c.ID, Revision: c.Revision, Name: c.DisplayName, Note: c.Notes}
	for _, m := range c.Methods {
		if m.Kind == "email" && out.Addr == "" {
			out.Addr = m.Value
		}
		if m.Kind == "phone" && out.Phone == "" {
			out.Phone = m.Value
		}
	}
	if len(c.Memberships) > 0 {
		out.Organization = c.Memberships[0].Organization
	}
	return out
}
func Serve(ctx context.Context, s *sqlite.Store, in io.Reader, out io.Writer) error {
	var req Request
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&req); err != nil {
		return err
	}
	resp := response{API: 1, Type: "response", RequestID: req.RequestID}
	data, err := handle(ctx, s, req)
	if err != nil {
		resp.Error = map[string]string{"code": "contacts_error", "message": err.Error()}
	} else {
		resp.OK = true
		resp.Data = data
	}
	return json.NewEncoder(out).Encode(resp)
}
func handle(ctx context.Context, s *sqlite.Store, req Request) (any, error) {
	if req.API != 1 || req.Type != "request" {
		return nil, fmt.Errorf("unsupported protocol")
	}
	switch req.Method {
	case "ping":
		return map[string]string{"message": "TideContacts ready"}, nil
	case "contacts.search", "contacts.list":
		var q struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal(req.Data, &q); err != nil {
			return nil, err
		}
		if len(q.Query) > 200 {
			return nil, fmt.Errorf("search is too long")
		}
		rows, err := s.List(ctx, "person", search.Query{Terms: strings.Fields(q.Query)}, "name")
		if err != nil {
			return nil, err
		}
		limit := q.Limit
		if limit <= 0 || limit > 200 {
			limit = 200
		}
		if req.Method == "contacts.search" {
			limit = min(limit, 50)
		}
		contacts := []Contact{}
		for _, r := range rows {
			if len(contacts) >= limit {
				break
			}
			c, err := s.Contact(ctx, r.ID)
			if err != nil {
				return nil, err
			}
			p := project(c)
			if p.Addr != "" {
				contacts = append(contacts, p)
			}
		}
		if req.Method == "contacts.search" {
			matches := []map[string]string{}
			for _, c := range contacts {
				matches = append(matches, map[string]string{"name": c.Name, "addr": c.Addr})
			}
			return map[string]any{"contacts": matches}, nil
		}
		return map[string]any{"contacts": contacts}, nil
	case "contacts.save":
		var input Contact
		if err := json.Unmarshal(req.Data, &input); err != nil {
			return nil, err
		}
		addr, err := mail.ParseAddress(strings.TrimSpace(input.Addr))
		if err != nil {
			return nil, fmt.Errorf("enter a valid email address")
		}
		if addr.Address != strings.TrimSpace(input.Addr) {
			return nil, fmt.Errorf("enter only the email address in Email")
		}
		c := domain.NewContact("person")
		if input.ID != "" {
			c, err = s.Contact(ctx, input.ID)
			if err != nil {
				return nil, err
			}
			if c.Deleted || c.Archived {
				return nil, fmt.Errorf("contact is no longer active")
			}
			if c.Revision != input.Revision {
				return nil, fmt.Errorf("contact changed; reopen before editing")
			}
		}
		c.DisplayName = strings.TrimSpace(input.Name)
		c.Notes = input.Note
		// Edit only the first email/phone; preserve additional methods and CRM fields.
		for kind, value := range map[string]string{"email": addr.Address, "phone": input.Phone} {
			found := false
			for i := range c.Methods {
				if c.Methods[i].Kind == kind {
					if value == "" {
						c.Methods = append(c.Methods[:i], c.Methods[i+1:]...)
					} else {
						c.Methods[i].Value = value
					}
					found = true
					break
				}
			}
			if !found && value != "" {
				c.Methods = append(c.Methods, domain.Method{Kind: kind, Value: value})
			}
		}
		if len(c.Memberships) == 0 && input.Organization != "" {
			c.Memberships = []domain.Membership{{Organization: input.Organization, Current: true}}
		} else if len(c.Memberships) > 0 && input.Organization != c.Memberships[0].Organization {
			c.Memberships[0].Organization = input.Organization
			c.Memberships[0].OrganizationID = ""
			if input.Organization == "" {
				c.Memberships = c.Memberships[1:]
			}
		}
		// Exact email duplicates require editing the existing person instead.
		matches, err := s.List(ctx, "person", search.Query{Terms: []string{addr.Address}}, "name")
		if err != nil {
			return nil, err
		}
		for _, r := range matches {
			if r.ID == c.ID {
				continue
			}
			other, err := s.Contact(ctx, r.ID)
			if err != nil {
				return nil, err
			}
			for _, m := range other.Methods {
				if m.Kind == "email" && strings.EqualFold(m.Value, addr.Address) {
					return nil, fmt.Errorf("that email already belongs to %s", other.DisplayName)
				}
			}
		}
		saved, err := s.SaveContacts(ctx, []domain.Contact{c}, "TideMail contact edit")
		if err != nil {
			return nil, err
		}
		return map[string]any{"contact": project(saved[0])}, nil
	}
	return nil, fmt.Errorf("unsupported method %q", req.Method)
}
