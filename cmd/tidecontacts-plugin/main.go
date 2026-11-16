package main

import (
	"context"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/demo"
	"github.com/allisonhere/tidecontacts/internal/mailbridge"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if os.Getenv("TIDECONTACTS_DEMO") == "1" {
		if os.Getenv("XDG_DATA_HOME") == "" || os.Getenv("XDG_CONFIG_HOME") == "" {
			return fmt.Errorf("demo requires isolated XDG paths")
		}
		paths = demo.Paths(paths)
	}
	s, err := sqlite.Open(ctx, paths.Database)
	if err != nil {
		return err
	}
	defer s.Close()
	return mailbridge.Serve(ctx, s, os.Stdin, os.Stdout)
}
