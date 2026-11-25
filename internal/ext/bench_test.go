package ext

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// BenchmarkRecordsResolve measures the host call an extension uses to look up
// many Community records at once, over address books of 1,000 and 10,000 people.
func BenchmarkRecordsResolve(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			store, err := sqlite.Open(context.Background(), filepath.Join(b.TempDir(), "b.db"))
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			var batch []domain.Contact
			var ids []string
			for i := 0; i < n; i++ {
				c := domain.NewContact("person")
				c.DisplayName = fmt.Sprintf("Person %05d", i)
				c.Methods = []domain.Method{{Kind: "email", Value: fmt.Sprintf("p%d@example.com", i)}}
				batch = append(batch, c)
				ids = append(ids, c.ID)
			}
			if _, err := store.SaveContacts(context.Background(), batch, "demo"); err != nil {
				b.Fatal(err)
			}
			s := &Session{ID: "bench-ext", Repo: store, Granted: NewPermissionSet("contacts.read")}
			args := []byte(fmt.Sprintf(`{"kind":"contact","ids":[%s]}`, quoteJoin(ids[:500])))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, e := s.Call(context.Background(), "records.resolve", args); e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}

func quoteJoin(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += `"` + id + `"`
	}
	return out
}
