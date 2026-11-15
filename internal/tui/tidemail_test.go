package tui

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTideMailPaletteFlow(t *testing.T) {
	dataRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataRoot)
	m, store := crmApp(t, 120, 35)
	m, cmd, handled := m.crmGlobal("tidemail-sync")
	if !handled || m.crm.prompt == nil {
		t.Fatal("sync prompt missing")
	}
	_ = cmd
	if _, err := m.crm.prompt.submit(&m, "relative.db"); err == nil {
		t.Fatal("relative path accepted")
	}
	m = do(m, "esc")
	if m.crm.prompt != nil || m.crm.busy {
		t.Fatal("cancel started sync")
	}
	path := filepath.Join(dataRoot, "tidemail", "mail.db")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE messages(id INTEGER PRIMARY KEY,message_id TEXT,subject TEXT,from_addr TEXT,to_addr TEXT,cc_addr TEXT,date INTEGER);
 INSERT INTO messages VALUES(1,'<ui@example.org>','UI sync','','','',1700000000)`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	m, cmd, _ = m.crmGlobal("tidemail-sync")
	if m.crm.prompt != nil || cmd == nil {
		t.Fatal("default cache was not discovered")
	}
	if !m.crm.busy {
		t.Fatal("sync should be busy")
	}
	msg := cmd().(crmDoneMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m, _, _ = m.updateCRM(msg)
	if m.crm.busy || !strings.Contains(m.feedback, "1 imported/updated") {
		t.Fatal(m.feedback)
	}
	last, err := store.Get(context.Background(), "tidemail-last-synced")
	if err != nil || last == "" {
		t.Fatal("sync status not saved")
	}
	m, cmd, _ = m.crmGlobal("tidemail-status")
	msg = cmd().(crmDoneMsg)
	if msg.err != nil || !strings.Contains(msg.note, last) {
		t.Fatal(msg)
	}
}
