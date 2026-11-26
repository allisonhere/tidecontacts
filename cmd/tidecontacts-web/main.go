// Command tidecontacts-web serves the local graphical clients: a loopback JSON
// API (and, optionally, the browser client) bound to one launch-specific token.
// The first stdout line is the only output: http://127.0.0.1:PORT/#session=TOKEN
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/demo"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/ext/observe"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"github.com/allisonhere/tidecontacts/internal/webapp"
	"github.com/allisonhere/tidecontacts/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tidecontacts-web:", err)
		os.Exit(1)
	}
}

func run() error {
	profile := flag.String("profile", "local", "local or demo (isolated fictional data)")
	apiOnly := flag.Bool("api-only", false, "serve only the JSON API, for native clients")
	assetDir := flag.String("assets", "", "serve the browser client from this directory instead of the embedded build")
	flag.Parse()
	if flag.NArg() > 0 || *profile != "local" && *profile != "demo" {
		return errors.New("usage: tidecontacts-web [--profile local|demo] [--api-only] [--assets DIR]")
	}
	var assets fs.FS
	if !*apiOnly {
		switch {
		case *assetDir != "":
			assets = os.DirFS(*assetDir)
		default:
			assets = web.Assets()
		}
		if assets == nil {
			return errors.New("this build has no browser client; build with `make web` or pass --assets, or use --api-only")
		}
	}

	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	isDemo := *profile == "demo"
	if isDemo {
		paths = demo.Paths(paths)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	store, err := sqlite.Open(ctx, paths.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	if isDemo {
		if err := demo.Seed(ctx, store); err != nil {
			return err
		}
	}
	bus := ext.NewBus()
	observed := observe.Wrap(store, bus, "web")
	dir, data, state := ext.PathsFor(paths.Config, paths.Database)
	provider, notice := ext.ProviderFromEnv()
	manager := ext.New(ext.Options{Dir: dir, DataDir: data, StateFile: state, Repo: app.CRM(observed), Bus: bus, Logger: logger, Entitlements: provider, Notice: notice})
	defer manager.Close()
	if err := manager.Discover(ctx); err != nil {
		logger.Warn("extension discovery failed", "error", err)
	} else {
		manager.Start()
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           webapp.New(observed, isDemo, listener.Addr().String(), token, assets, manager),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("http://%s/#session=%s\n", listener.Addr(), token)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
