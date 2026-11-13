package domain

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"math"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Contact is an aggregate. Child collections are stored in normalized tables.
type Contact struct {
	Incoming                                                  []Relationship `json:"-"`
	Members                                                   []Relationship `json:"-"`
	ID, Kind, DisplayName                                     string
	Given, Middle, Family, Prefix, Suffix, Nickname, Phonetic string
	Birthday, Anniversary, Biography, Notes, Avatar           string
	Aliases                                                   []string
	Website, Industry, Source                                 string
	Favorite, Archived, Deleted                               bool
	CreatedAt, UpdatedAt                                      string
	Revision                                                  int
	Methods                                                   []Method
	Memberships                                               []Membership
	Relationships                                             []Relationship
	Tags                                                      []string
	Fields                                                    []CustomField
	Extensions                                                map[string]json.RawMessage
}
type Method struct {
	ID, Kind, Label, Value, Notes, Source, Verification string
	Preferred                                           bool
	Address                                             Address
}
type Address struct{ POBox, Extended, Street, City, Region, PostalCode, Country string }

func (a Address) String() string {
	return strings.Join([]string{a.POBox, a.Extended, a.Street, a.City, a.Region, a.PostalCode, a.Country}, "; ")
}

type Membership struct {
	ID, OrganizationID, Organization, Title, Department, Start, End string
	Current                                                         bool
}
type Relationship struct{ ID, TargetID, TargetName, Type, Notes string }
type CustomField struct {
	Name, Type, Value string
	Options           []string
}
type History struct {
	Revision             int
	At, Action, Snapshot string
}
type Summary struct {
	ID, Kind, Name, Organization, Tags, UpdatedAt, CreatedAt string
	Favorite, Archived                                       bool
	Revision                                                 int
	SearchText                                               string
}

func NewContact(kind string) Contact {
	return Contact{ID: uuid.NewString(), Kind: kind, Source: "manual"}
}
func (c *Contact) Normalize() {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	if c.Kind == "" {
		c.Kind = "person"
	}
	if c.Source == "" {
		c.Source = "manual"
	}
	c.DisplayName = strings.TrimSpace(c.DisplayName)
	if c.DisplayName == "" {
		c.DisplayName = strings.TrimSpace(strings.Join([]string{c.Prefix, c.Given, c.Middle, c.Family, c.Suffix}, " "))
		c.DisplayName = strings.Join(strings.Fields(c.DisplayName), " ")
	}
	if c.DisplayName == "" {
		c.DisplayName = c.Nickname
	}
	if c.DisplayName == "" && len(c.Memberships) > 0 {
		c.DisplayName = c.Memberships[0].Organization
	}
	if c.DisplayName == "" && len(c.Methods) > 0 {
		c.DisplayName = c.Methods[0].Value
	}
	seen := map[string]bool{}
	tags := []string{}
	for _, t := range c.Tags {
		t = strings.TrimSpace(t)
		if t != "" && !seen[strings.ToLower(t)] {
			tags = append(tags, t)
			seen[strings.ToLower(t)] = true
		}
	}
	c.Tags = tags
	for i := range c.Methods {
		m := &c.Methods[i]
		if m.ID == "" {
			m.ID = uuid.NewString()
		}
		m.Value = strings.TrimSpace(m.Value)
		if m.Kind == "email" {
			if a, e := mail.ParseAddress(m.Value); e == nil {
				m.Value = a.Address
			}
		}
		if m.Verification == "" {
			m.Verification = "unknown"
		}
	}
	for i := range c.Memberships {
		if c.Memberships[i].ID == "" {
			c.Memberships[i].ID = uuid.NewString()
		}
	}
	for i := range c.Relationships {
		if c.Relationships[i].ID == "" {
			c.Relationships[i].ID = uuid.NewString()
		}
	}
}
func DateValid(s string) bool {
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "--") {
		_, e := time.Parse("2006-01-02", "2000-"+strings.TrimPrefix(s, "--"))
		return e == nil
	}
	_, e := time.Parse("2006-01-02", s)
	return e == nil
}
func URLValid(s string) bool {
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}
func (c Contact) Validate() error {
	if _, e := uuid.Parse(c.ID); e != nil {
		return fmt.Errorf("invalid record UUID")
	}
	if c.Kind != "person" && c.Kind != "organization" {
		return fmt.Errorf("kind must be person or organization")
	}
	if strings.TrimSpace(c.DisplayName) == "" {
		return fmt.Errorf("provide a display name, nickname or contact method")
	}
	if !DateValid(c.Birthday) || !DateValid(c.Anniversary) {
		return fmt.Errorf("birthday/anniversary must be YYYY-MM-DD or --MM-DD")
	}
	if c.Website != "" && !URLValid(c.Website) {
		return fmt.Errorf("website must be an http(s) URL")
	}
	preferred := map[string]bool{}
	for i, m := range c.Methods {
		if m.Kind != "email" && m.Kind != "phone" && m.Kind != "website" && m.Kind != "social" && m.Kind != "messaging" && m.Kind != "address" {
			return fmt.Errorf("method %d: unknown kind %q", i+1, m.Kind)
		}
		if m.Kind == "email" {
			if _, e := mail.ParseAddress(m.Value); e != nil {
				return fmt.Errorf("method %d: invalid email", i+1)
			}
		}
		if m.Kind == "website" && !URLValid(m.Value) {
			return fmt.Errorf("method %d: website must be http(s)", i+1)
		}
		if m.Kind != "address" && m.Value == "" {
			return fmt.Errorf("method %d needs a value", i+1)
		}
		if m.Kind == "address" && strings.Trim(m.Address.String(), "; ") == "" {
			return fmt.Errorf("address %d is empty", i+1)
		}
		if m.Preferred && preferred[m.Kind] {
			return fmt.Errorf("only one preferred %s method is allowed", m.Kind)
		}
		if m.Preferred {
			preferred[m.Kind] = true
		}
		if m.Verification != "unknown" && m.Verification != "verified" && m.Verification != "unverified" {
			return fmt.Errorf("method %d: verification must be unknown, verified or unverified", i+1)
		}
	}
	for _, m := range c.Memberships {
		if c.Kind != "person" {
			return fmt.Errorf("only people can have organization memberships")
		}
		if m.OrganizationID == "" && strings.TrimSpace(m.Organization) == "" {
			return fmt.Errorf("membership needs an organization")
		}
		if !DateValid(m.Start) || !DateValid(m.End) {
			return fmt.Errorf("membership dates must be YYYY-MM-DD")
		}
		if m.Start != "" && m.End != "" && m.End < m.Start {
			return fmt.Errorf("membership end precedes start")
		}
	}
	names := map[string]bool{}
	for _, f := range c.Fields {
		if strings.TrimSpace(f.Name) == "" || names[strings.ToLower(f.Name)] {
			return fmt.Errorf("custom field names must be nonempty and unique")
		}
		names[strings.ToLower(f.Name)] = true
		if e := f.Validate(); e != nil {
			return e
		}
	}
	for _, r := range c.Relationships {
		if strings.TrimSpace(r.Type) == "" || r.TargetID == "" && strings.TrimSpace(r.TargetName) == "" {
			return fmt.Errorf("relationship requires a target and type")
		}
		if r.TargetID == c.ID {
			return fmt.Errorf("cannot relate a contact to itself")
		}
	}
	return nil
}
func (f CustomField) Validate() error {
	valid := true
	switch f.Type {
	case "text":
	case "number":
		n, e := strconv.ParseFloat(f.Value, 64)
		valid = e == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
	case "date":
		valid = f.Value != "" && DateValid(f.Value)
	case "boolean":
		valid = f.Value == "true" || f.Value == "false"
	case "url":
		valid = URLValid(f.Value)
	case "enum":
		valid = false
		for _, v := range f.Options {
			if v == f.Value {
				valid = true
			}
		}
	default:
		return fmt.Errorf("field %s: unsupported type %q", f.Name, f.Type)
	}
	if !valid {
		return fmt.Errorf("field %s: invalid %s value %q", f.Name, f.Type, f.Value)
	}
	return nil
}
func NormalizeMatch(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '@' || r == '.' {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// SafeText removes terminal controls from external content without altering stored values.
func SafeText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}
