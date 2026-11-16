package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/demo"
	"github.com/allisonhere/tidecontacts/internal/mailbridge"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error { return serve(os.Stdin, os.Stdout) }
func serve(in io.Reader, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var req mailbridge.Request
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&req); err != nil {
		return err
	}
	if req.API != 1 || req.Type != "request" {
		return fmt.Errorf("unsupported plugin protocol")
	}
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	forcedDemo := os.Getenv("TIDECONTACTS_DEMO") == "1"
	if forcedDemo && (os.Getenv("XDG_DATA_HOME") == "" || os.Getenv("XDG_CONFIG_HOME") == "") {
		return fmt.Errorf("demo requires isolated XDG paths")
	}
	useDemo := forcedDemo || req.Settings["data_profile"] == "Demo contacts"
	profile := "Real contacts"
	if useDemo {
		paths = demo.Paths(paths)
		profile = "Demo contacts"
	}
	if req.Method == "plugin.test" {
		message, err := checkConnection(ctx, paths.Database, profile)
		resp := map[string]any{"api": 1, "type": "response", "request_id": req.RequestID, "ok": err == nil}
		if err != nil {
			resp["error"] = map[string]string{"code": "connection_failed", "message": err.Error()}
		} else {
			resp["data"] = map[string]any{"message": message, "profile": profile}
		}
		return json.NewEncoder(out).Encode(resp)
	}
	s, err := sqlite.Open(ctx, paths.Database)
	if err != nil {
		return err
	}
	defer s.Close()
	if useDemo {
		if err := demo.Seed(ctx, s); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return mailbridge.Serve(ctx, s, bytes.NewReader(raw), out)
}
func checkConnection(ctx context.Context, path, profile string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "Ready · " + profile + " · database will be created when opened", nil
	} else if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return "", err
	}
	defer db.Close()
	var count int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM entities WHERE kind='person' AND deleted=0 AND archived=0").Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Connected · %s · %d people", profile, count), nil
}
