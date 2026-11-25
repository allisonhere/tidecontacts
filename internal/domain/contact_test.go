package domain

import "testing"

func TestValidation(t *testing.T) {
	c := NewContact("person")
	c.Nickname = "Handle"
	c.Normalize()
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"2024-02-29", "--02-29", ""} {
		if !DateValid(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"2023-02-29", "--13-01", "x"} {
		if DateValid(s) {
			t.Fatal(s)
		}
	}
	for _, f := range []CustomField{{Name: "x", Type: "number", Value: "NaN"}, {Name: "x", Type: "url", Value: "javascript:test"}, {Name: "x", Type: "boolean", Value: "yes"}, {Name: "x", Type: "enum", Value: "other", Options: []string{"one"}}} {
		if f.Validate() == nil {
			t.Fatal(f)
		}
	}
}

// TestNormalizeWebsiteAddsScheme guards against sources such as Google
// Contacts that store a website as a bare domain with no "http(s)" scheme,
// which otherwise fails Validate and rejects the whole import.
func TestNormalizeWebsiteAddsScheme(t *testing.T) {
	c := NewContact("person")
	c.Nickname = "Handle"
	c.Website = "example.com"
	c.Methods = append(c.Methods, Method{Kind: "website", Value: "www.example.org/profile"})
	c.Normalize()
	if c.Website != "https://example.com" {
		t.Fatalf("website scheme not added: %q", c.Website)
	}
	if c.Methods[0].Value != "https://www.example.org/profile" {
		t.Fatalf("method website scheme not added: %q", c.Methods[0].Value)
	}
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
}
