package mcpserver

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func BenchmarkMCPConcurrentContactPages(b *testing.B) {
	ctx := b.Context()
	store, err := sqlite.Open(ctx, filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SaveMCPProfile(ctx, app.MCPProfile{Name: "benchmark", Enabled: true, Scopes: []string{"contacts.read"}}); err != nil {
		b.Fatal(err)
	}
	for page := range 10 {
		rows := make([]domain.Contact, 100)
		for i := range rows {
			rows[i] = domain.NewContact("person")
			rows[i].DisplayName = fmt.Sprintf("Fictional contact %04d", page*len(rows)+i)
		}
		if _, err := store.SaveContacts(app.WithoutEvents(ctx), rows, "benchmark"); err != nil {
			b.Fatal(err)
		}
	}
	server, err := New(ctx, store, store, "benchmark", nil)
	if err != nil {
		b.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.sdk.Connect(ctx, st, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "benchmark", Version: "1"}, nil)
	s, err := client.Connect(ctx, ct, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	b.ReportAllocs()
	for b.Loop() {
		var wg sync.WaitGroup
		var failures [4]error
		for i := range failures {
			wg.Go(func() {
				res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "contact_search", Arguments: map[string]any{"query": "Fictional", "limit": 20}})
				if err == nil && (res.IsError || res.StructuredContent == nil) {
					err = fmt.Errorf("tool returned no contact page")
				}
				failures[i] = err
			})
		}
		wg.Wait()
		for _, err := range failures {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportMetric(4, "requests/op")
}
