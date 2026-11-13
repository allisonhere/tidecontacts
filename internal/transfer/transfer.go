// Package transfer provides preview-first imports and no-clobber exports.
package transfer

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/search"
	"github.com/emersion/go-vcard"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var Columns = []string{"ignore", "display_name", "given", "middle", "family", "prefix", "suffix", "nickname", "email", "phone", "website", "organization", "title", "department", "notes", "birthday", "anniversary", "tags", "street", "city", "region", "postal_code", "country", "kind", "tidecontacts_json"}

type File struct {
	Format    string
	Headers   []string
	Rows      [][]string
	Cards     []domain.Contact
	Mapping   []string
	HasHeader bool
}
type Record struct {
	Contact          domain.Contact
	Error, Duplicate string
	Include          bool
	Row              int
}
type Preview struct{ Records []Record }

func DetectHeader(row []string) bool {
	for _, v := range row {
		if DefaultColumn(v) != "ignore" {
			return true
		}
	}
	return false
}
func DefaultColumn(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "_", "-", "_").Replace(s)
	switch s {
	case "name", "full_name", "formatted_name":
		return "display_name"
	case "first_name", "given_name":
		return "given"
	case "last_name", "surname", "family_name":
		return "family"
	case "e_mail", "email_address", "e_mail_1_value":
		return "email"
	case "phone_number", "mobile", "phone_1_value":
		return "phone"
	case "company", "organization_name":
		return "organization"
	case "note":
		return "notes"
	case "categories", "groups":
		return "tags"
	}
	for _, v := range Columns {
		if s == v {
			return v
		}
	}
	return "ignore"
}
func ReadFile(path string) (File, error) {
	f, e := os.Open(path)
	if e != nil {
		return File{}, e
	}
	defer func() { _ = f.Close() }()
	st, e := f.Stat()
	if e != nil {
		return File{}, e
	}
	if !st.Mode().IsRegular() {
		return File{}, fmt.Errorf("choose a regular file")
	}
	if st.Size() > 50<<20 {
		return File{}, fmt.Errorf("import exceeds 50 MiB; split it into smaller files")
	}
	b, e := io.ReadAll(io.LimitReader(f, (50<<20)+1))
	if e != nil {
		return File{}, e
	}
	if len(b) > 50<<20 {
		return File{}, fmt.Errorf("import exceeds 50 MiB")
	}
	return Read(b)
}
func Read(data []byte) (File, error) {
	data = bytes.TrimPrefix(data, []byte{239, 187, 191})
	f := File{}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("BEGIN:VCARD")) {
		f.Format = "vcard"
		d := vcard.NewDecoder(bytes.NewReader(data))
		for {
			card, e := d.Decode()
			if e == io.EOF {
				break
			}
			if e != nil {
				return f, e
			}
			c, e := fromCard(card)
			if e != nil {
				return f, e
			}
			f.Cards = append(f.Cards, c)
		}
		return f, nil
	}
	f.Format = "csv"
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	rows, e := r.ReadAll()
	if e != nil {
		return f, e
	}
	if len(rows) == 0 {
		return f, fmt.Errorf("empty import file")
	}
	f.Rows = rows
	f.HasHeader = DetectHeader(rows[0])
	f.Headers = rows[0]
	f.Mapping = make([]string, len(f.Headers))
	for i, h := range f.Headers {
		f.Mapping[i] = DefaultColumn(h)
	}
	return f, nil
}
func SplitTags(s string) []string {
	r := csv.NewReader(strings.NewReader(s))
	v, e := r.Read()
	if e != nil {
		return strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' })
	}
	for i := range v {
		v[i] = strings.TrimSpace(v[i])
	}
	return v
}
func fromRow(row, mapping []string) (domain.Contact, error) {
	if len(row) != len(mapping) {
		return domain.Contact{}, fmt.Errorf("row has %d columns; mapping expects %d", len(row), len(mapping))
	}
	c := domain.NewContact("person")
	c.Source = "csv"
	org, title, department := "", "", ""
	address := domain.Address{}
	for i, key := range mapping {
		if i >= len(row) {
			continue
		}
		v := strings.TrimSpace(row[i])
		if key == "tidecontacts_json" && v != "" {
			if e := json.Unmarshal([]byte(v), &c); e != nil {
				return c, fmt.Errorf("invalid TideContacts extension: %w", e)
			}
			return c, nil
		}
	}
	for i, key := range mapping {
		if i >= len(row) {
			continue
		}
		v := strings.TrimSpace(row[i])
		if v == "" {
			continue
		}
		switch key {
		case "display_name":
			c.DisplayName = v
		case "given":
			c.Given = v
		case "middle":
			c.Middle = v
		case "family":
			c.Family = v
		case "prefix":
			c.Prefix = v
		case "suffix":
			c.Suffix = v
		case "nickname":
			c.Nickname = v
		case "kind":
			c.Kind = v
		case "email", "phone", "website":
			c.Methods = append(c.Methods, domain.Method{Kind: key, Value: v, Source: "csv"})
		case "organization":
			org = v
		case "title":
			title = v
		case "department":
			department = v
		case "notes":
			c.Notes = v
		case "birthday":
			c.Birthday = normalizeDate(v)
		case "anniversary":
			c.Anniversary = normalizeDate(v)
		case "tags":
			c.Tags = SplitTags(v)
		case "street":
			address.Street = v
		case "city":
			address.City = v
		case "region":
			address.Region = v
		case "postal_code":
			address.PostalCode = v
		case "country":
			address.Country = v
		}
	}
	if strings.Trim(address.String(), "; ") != "" {
		c.Methods = append(c.Methods, domain.Method{Kind: "address", Address: address, Source: "csv"})
	}
	if org != "" && c.Kind == "person" {
		c.Memberships = append(c.Memberships, domain.Membership{Organization: org, Title: title, Department: department, Current: true})
	}
	return c, nil
}
func normalizeDate(s string) string {
	for _, format := range []string{"20060102", "2006-01-02", "--0102"} {
		if d, e := time.Parse(format, s); e == nil {
			if strings.HasPrefix(s, "--") {
				return "--" + d.Format("01-02")
			}
			return d.Format("2006-01-02")
		}
	}
	return s
}
func fromCard(card vcard.Card) (domain.Contact, error) {
	c := domain.NewContact("person")
	c.Source = "vcard"
	if raw := card.Value("X-TIDECONTACTS"); raw != "" {
		if e := json.Unmarshal([]byte(raw), &c); e != nil {
			return c, fmt.Errorf("invalid X-TIDECONTACTS: %w", e)
		}
		return c, nil
	}
	if card.Value(vcard.FieldKind) == "org" {
		c.Kind = "organization"
	}
	c.DisplayName = card.Value(vcard.FieldFormattedName)
	if n := card.Name(); n != nil {
		c.Given, c.Middle, c.Family, c.Prefix, c.Suffix = n.GivenName, n.AdditionalName, n.FamilyName, n.HonorificPrefix, n.HonorificSuffix
	}
	c.Nickname = card.Value(vcard.FieldNickname)
	c.Notes = card.Value(vcard.FieldNote)
	c.Birthday = normalizeDate(card.Value(vcard.FieldBirthday))
	c.Anniversary = normalizeDate(card.Value(vcard.FieldAnniversary))
	c.Tags = card.Categories()
	c.Avatar = card.Value(vcard.FieldPhoto)
	for _, pair := range [][2]string{{vcard.FieldEmail, "email"}, {vcard.FieldTelephone, "phone"}, {vcard.FieldURL, "website"}, {vcard.FieldIMPP, "messaging"}, {"X-SOCIALPROFILE", "social"}} {
		for _, field := range card[pair[0]] {
			if field == nil {
				continue
			}
			value := field.Value
			if pair[1] == "phone" {
				value = strings.TrimPrefix(value, "tel:")
			}
			preferred := field.Params.Get("PREF") == "1" || strings.Contains(strings.ToLower(strings.Join(field.Params["TYPE"], ",")), "pref")
			c.Methods = append(c.Methods, domain.Method{Kind: pair[1], Label: strings.Join(field.Params["TYPE"], ","), Value: value, Preferred: preferred, Source: "vcard"})
		}
	}
	for _, a := range card.Addresses() {
		c.Methods = append(c.Methods, domain.Method{Kind: "address", Label: strings.Join(a.Params["TYPE"], ","), Preferred: a.Params.Get("PREF") == "1", Source: "vcard", Address: domain.Address{POBox: a.PostOfficeBox, Extended: a.ExtendedAddress, Street: a.StreetAddress, City: a.Locality, Region: a.Region, PostalCode: a.PostalCode, Country: a.Country}})
	}
	for _, org := range card[vcard.FieldOrganization] {
		if org != nil && org.Value != "" && c.Kind == "person" {
			parts := strings.SplitN(org.Value, ";", 2)
			m := domain.Membership{Organization: parts[0], Title: card.Value(vcard.FieldTitle), Current: true}
			if len(parts) > 1 {
				m.Department = parts[1]
			}
			c.Memberships = append(c.Memberships, m)
		}
	}
	raw, e := json.Marshal(card)
	if e != nil {
		return c, e
	}
	c.Extensions = map[string]json.RawMessage{"vcard": raw}
	return c, nil
}
func BuildPreview(ctx context.Context, repo app.Contacts, f File) (Preview, error) {
	var preview Preview
	incoming := append([]domain.Contact(nil), f.Cards...)
	rowNumbers := []int{}
	if f.Format == "csv" {
		start := 0
		if f.HasHeader {
			start = 1
		}
		for i := start; i < len(f.Rows); i++ {
			c, e := fromRow(f.Rows[i], f.Mapping)
			if e != nil {
				preview.Records = append(preview.Records, Record{Row: i + 1, Error: e.Error()})
				continue
			}
			incoming = append(incoming, c)
			rowNumbers = append(rowNumbers, i+1)
		}
	}
	// Import always creates new UUIDs; map internal references within this batch.
	idMap := map[string]string{}
	for _, c := range incoming {
		if c.ID != "" {
			idMap[c.ID] = uuid.NewString()
		}
	}
	existing := map[string]string{}
	names := map[string]string{}
	for _, kind := range []string{"person", "organization"} {
		for _, query := range []string{"", "is:archived", "is:deleted"} {
			q, _ := search.Parse(query)
			list, e := repo.List(ctx, kind, q, "name")
			if e != nil {
				return preview, e
			}
			for _, s := range list {
				names[strings.ToLower(s.Name)] = s.Name
				c, e := repo.Contact(ctx, s.ID)
				if e != nil {
					return preview, e
				}
				for _, m := range c.Methods {
					if m.Kind == "email" || m.Kind == "phone" {
						existing[m.Kind+":"+domain.NormalizeMatch(m.Value)] = s.Name
					}
				}
			}
		}
	}
	for i, c := range incoming {
		if e := ctx.Err(); e != nil {
			return preview, e
		}
		if id, ok := idMap[c.ID]; ok {
			c.ID = id
		} else {
			c.ID = uuid.NewString()
		}
		c.Revision = 0
		c.CreatedAt = ""
		c.UpdatedAt = ""
		c.Deleted = false
		for j := range c.Methods {
			c.Methods[j].ID = ""
		}
		for j := range c.Memberships {
			m := &c.Memberships[j]
			m.ID = ""
			if id, ok := idMap[m.OrganizationID]; ok {
				m.OrganizationID = id
			} else {
				m.OrganizationID = ""
			}
		}
		for j := range c.Relationships {
			r := &c.Relationships[j]
			r.ID = ""
			if id, ok := idMap[r.TargetID]; ok {
				r.TargetID = id
			} else {
				r.TargetID = ""
			}
		}
		c.Normalize()
		record := Record{Contact: c, Row: i + 1, Include: true}
		if i < len(rowNumbers) {
			record.Row = rowNumbers[i]
		}
		if e := c.Validate(); e != nil {
			record.Error = e.Error()
			record.Include = false
		}
		if name := names[strings.ToLower(c.DisplayName)]; name != "" {
			record.Duplicate = "same name: " + name
		}
		for _, m := range c.Methods {
			if name := existing[m.Kind+":"+domain.NormalizeMatch(m.Value)]; name != "" {
				record.Duplicate = "matching " + m.Kind + ": " + name
			}
			if m.Kind == "email" || m.Kind == "phone" {
				existing[m.Kind+":"+domain.NormalizeMatch(m.Value)] = c.DisplayName
			}
		}
		names[strings.ToLower(c.DisplayName)] = c.DisplayName
		if record.Duplicate != "" {
			record.Include = false
		}
		preview.Records = append(preview.Records, record)
	}
	return preview, nil
}
func (p Preview) Selected() []domain.Contact {
	out := []domain.Contact{}
	for _, r := range p.Records {
		if r.Include && r.Error == "" {
			out = append(out, r.Contact)
		}
	}
	return out
}
func ExportFile(path, format string, contacts []domain.Contact) error {
	dir := filepath.Dir(path)
	f, e := os.CreateTemp(dir, ".tidecontacts-export-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	defer func() { _ = f.Close() }()
	if e = Write(f, format, contacts); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	// Linking atomically publishes a complete file and fails if the destination exists.
	if e = os.Link(name, path); e != nil {
		return fmt.Errorf("export not published (destination must not exist): %w", e)
	}
	return nil
}
func Write(w io.Writer, format string, contacts []domain.Contact) error {
	switch format {
	case "csv":
		out := csv.NewWriter(w)
		if e := out.Write([]string{"display_name", "given", "family", "email", "phone", "organization", "notes", "tags", "tidecontacts_json"}); e != nil {
			return e
		}
		for _, c := range contacts {
			email, phone, org := "", "", ""
			for _, m := range c.Methods {
				if m.Kind == "email" {
					email = m.Value
				}
				if m.Kind == "phone" {
					phone = m.Value
				}
			}
			if len(c.Memberships) > 0 {
				org = c.Memberships[0].Organization
			}
			raw, e := json.Marshal(c)
			if e != nil {
				return e
			}
			if e = out.Write([]string{c.DisplayName, c.Given, c.Family, email, phone, org, c.Notes, strings.Join(c.Tags, ","), string(raw)}); e != nil {
				return e
			}
		}
		out.Flush()
		return out.Error()
	case "vcard":
		enc := vcard.NewEncoder(w)
		for _, c := range contacts {
			card := vcard.Card{}
			if raw, ok := c.Extensions["vcard"]; ok {
				if e := json.Unmarshal(raw, &card); e != nil {
					return e
				}
			}
			for _, key := range []string{"N", "FN", "EMAIL", "TEL", "ADR", "URL", "IMPP", "X-SOCIALPROFILE", "ORG", "TITLE", "NOTE", "BDAY", "ANNIVERSARY", "CATEGORIES", "NICKNAME", "PHOTO"} {
				delete(card, key)
			}
			card.SetValue("VERSION", "4.0")
			card.SetValue("UID", c.ID)
			card.SetValue("FN", c.DisplayName)
			if c.Kind == "organization" {
				card.SetValue("KIND", "org")
			}
			card.SetName(&vcard.Name{FamilyName: c.Family, GivenName: c.Given, AdditionalName: c.Middle, HonorificPrefix: c.Prefix, HonorificSuffix: c.Suffix})
			for key, value := range map[string]string{"NOTE": c.Notes, "BDAY": c.Birthday, "ANNIVERSARY": c.Anniversary, "NICKNAME": c.Nickname, "PHOTO": c.Avatar} {
				if value != "" {
					card.SetValue(key, value)
				}
			}
			card.SetCategories(c.Tags)
			for _, m := range c.Methods {
				params := vcard.Params{}
				if m.Label != "" {
					params["TYPE"] = []string{m.Label}
				}
				if m.Preferred {
					params["PREF"] = []string{"1"}
				}
				field := &vcard.Field{Value: m.Value, Params: params}
				if m.Kind == "address" {
					a := m.Address
					card.AddAddress(&vcard.Address{Field: field, PostOfficeBox: a.POBox, ExtendedAddress: a.Extended, StreetAddress: a.Street, Locality: a.City, Region: a.Region, PostalCode: a.PostalCode, Country: a.Country})
				} else {
					key := map[string]string{"email": "EMAIL", "phone": "TEL", "website": "URL", "social": "X-SOCIALPROFILE", "messaging": "IMPP"}[m.Kind]
					card.Add(key, field)
				}
			}
			for _, m := range c.Memberships {
				card.AddValue("ORG", m.Organization+";"+m.Department)
				if m.Title != "" {
					card.AddValue("TITLE", m.Title)
				}
			}
			raw, e := json.Marshal(c)
			if e != nil {
				return e
			}
			card.SetValue("X-TIDECONTACTS", string(raw))
			if e = enc.Encode(card); e != nil {
				return e
			}
		}
		return nil
	default:
		return fmt.Errorf("export format must be csv or vcard")
	}
}
