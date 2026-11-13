package search

import "testing"

func TestParser(t *testing.T) {
	q, e := Parse(`alex org:"Riverside Studio" tag:developer @github.com is:favorite has:phone`)
	if e != nil {
		t.Fatal(e)
	}
	if len(q.Terms) != 2 || len(q.Filters) != 4 || q.Filters[0].Value != "Riverside Studio" {
		t.Fatal(q)
	}
	for _, s := range []string{`org:"oops`, `tag:`, `is:wrong`, `nope:value`, `has:wrong`, `abc\`} {
		if _, e := Parse(s); e == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	if _, e := Parse(`"Robert'); DROP TABLE people;--"`); e != nil {
		t.Fatal(e)
	}
}
