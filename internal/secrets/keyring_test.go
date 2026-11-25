package secrets

import "testing"

// TestStoreLookupClearRoundtrip exercises the real keychain when one is
// available in the test environment, and otherwise verifies the documented
// fallback: Store reports false, Lookup returns "", Clear is a silent no-op.
func TestStoreLookupClearRoundtrip(t *testing.T) {
	const key, value = "test:roundtrip", "s3cr3t"
	usable, _ := Usable()
	stored := Store(key, value)
	if stored != usable {
		t.Fatalf("Store()=%v, want %v (Usable()=%v)", stored, usable, usable)
	}
	got := Lookup(key)
	if usable && got != value {
		t.Fatalf("Lookup()=%q, want %q", got, value)
	}
	if !usable && got != "" {
		t.Fatalf("Lookup() on unavailable keychain = %q, want \"\"", got)
	}
	Clear(key)
	if usable && Lookup(key) != "" {
		t.Fatal("Clear did not remove the stored secret")
	}
}

func TestDisabledViaEnv(t *testing.T) {
	t.Setenv("TIDECONTACTS_DISABLE_KEYRING", "1")
	ok, reason := usableFor("linux")
	if ok {
		t.Fatal("expected keyring to report unusable when disabled")
	}
	if reason == "" {
		t.Fatal("expected a reason when disabled")
	}
}
