package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsProfilesAndConnection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("TIDECONTACTS_DEMO", "")
	invoke := func(method, profile string) map[string]any {
		t.Helper()
		req := map[string]any{"api": 1, "type": "request", "request_id": "test", "method": method, "settings": map[string]any{"data_profile": profile}, "data": map[string]any{"query": ""}}
		raw, _ := json.Marshal(req)
		var out bytes.Buffer
		if err := serve(bytes.NewReader(raw), &out); err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["ok"] != true {
			t.Fatal(out.String())
		}
		return result
	}
	result := invoke("plugin.test", "Real contacts")
	if !strings.Contains(result["data"].(map[string]any)["message"].(string), "Ready") {
		t.Fatal(result)
	}
	live := filepath.Join(root, "data", "tidecontacts", "tidecontacts.db")
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatal("connection test created real db")
	}
	result = invoke("contacts.list", "Demo contacts")
	contacts := result["data"].(map[string]any)["contacts"].([]any)
	if len(contacts) != 6 {
		t.Fatal(len(contacts))
	}
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatal("demo opened real db")
	}
	result = invoke("plugin.test", "Demo contacts")
	if !strings.Contains(result["data"].(map[string]any)["message"].(string), "Connected") {
		t.Fatal(result)
	}
	t.Setenv("TIDECONTACTS_DEMO", "1")
	invoke("contacts.list", "Real contacts")
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Fatal("forced demo escaped")
	}
}
