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
