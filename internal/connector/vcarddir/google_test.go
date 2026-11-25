package vcarddir

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/tidecontacts/internal/connector/googlecontacts"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

// TestPrepareGoogleWithoutStoredPasswordFails forces the keychain
// unavailable (deterministic regardless of the host running the test) and
// checks that refreshing a Google book with no saved password fails clearly
// instead of attempting a network call with an empty password.
func TestPrepareGoogleWithoutStoredPasswordFails(t *testing.T) {
	t.Setenv("TIDECONTACTS_DISABLE_KEYRING", "1")
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	path := googlecontacts.Path("alex@example.com")
	_, _, err = Prepare(ctx, path, store)
	if err == nil || !strings.Contains(err.Error(), "no saved app password") {
		t.Fatalf("expected a no-saved-password error, got %v", err)
	}
}

// TestConnectGoogleReportsUnreachableAccount checks that a connect attempt
// against an email with no live account fails with the fetch error rather
// than silently storing a password for an account that was never validated.
func TestConnectGoogleReportsUnreachableAccount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // an already-cancelled context stands in for "unreachable" without a real network call
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	_, _, err = ConnectGoogle(ctx, "alex@example.com", "an-app-password", store)
	if err == nil {
		t.Fatal("expected ConnectGoogle to fail for a cancelled context")
	}
}
