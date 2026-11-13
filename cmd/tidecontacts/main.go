package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/allisonhere/tidecontacts/internal/config"
	"github.com/allisonhere/tidecontacts/internal/diagnostics"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"github.com/allisonhere/tidecontacts/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

func main() {
	if err := run(); err != nil {
		if errors.As(err, &errDue{}) {
			os.Exit(3)
		}
		fmt.Fprintln(os.Stderr, "tidecontacts:", err)
		os.Exit(1)
	}
}
func run() (result error) {
	check := flag.Bool("check", false, "initialize and check configuration/database without a terminal")
	version := flag.Bool("version", false, "show version")
	due := flag.Bool("due", false, "print reminders, follow-ups and commitments that are due, then exit (status 3 when something is due)")
	notify := flag.Bool("notify", false, "send each newly due reminder to the desktop notification service once, then exit")
	flag.Parse()
	if *version {
		fmt.Println("tidecontacts phase3")
		return nil
	}
	if flag.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flag.Arg(0))
	}
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	interactive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("TERM") != "dumb" && os.Getenv("TERM") != ""
	if *due || *notify {
		return report(paths.Database, *notify)
	}
	if !*check && !interactive {
		fmt.Println("TideContacts Phase 3 — run in an interactive terminal, or use --check, --due or --notify.")
		return nil
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	var store *sqlite.Store
	var logFile *os.File
	var logger *slog.Logger
	load := func() tea.Msg {
		defer wg.Done()
		c, e := config.Load(paths.Config)
		if e != nil {
			return tui.Ready{Err: e}
		}
		logger, logFile, e = diagnostics.Open(paths.Log, c.LogLevel)
		if e != nil {
			return tui.Ready{Err: e}
		}
		logger.Info("starting", "database", paths.Database)
		store, e = sqlite.Open(ctx, paths.Database)
		if e != nil {
			return tui.Ready{Err: e}
		}
		raw, e := store.Get(ctx, "pane-widths")
		if e != nil {
			return tui.Ready{Err: e}
		}
		if raw != "" {
			var p tui.Preferences
			if e = json.Unmarshal([]byte(raw), &p); e != nil {
				return tui.Ready{Err: fmt.Errorf("invalid saved layout: %w", e)}
			}
			c.Layout.Left, c.Layout.Right = p.Left, p.Right
			if e = c.Validate(); e != nil {
				return tui.Ready{Err: e}
			}
		}
		return tui.Ready{Config: c, Repo: store, Context: ctx}
	}
	wg.Add(1)
	// Start once before Run: quitting during initialization still joins and closes resources.
	loaded := make(chan tea.Msg, 1)
	go func() { loaded <- load() }()
	defer func() {
		cancel()
		wg.Wait()
		if store != nil {
			result = errors.Join(result, store.Close())
		}
		if logger != nil {
			logger.Info("shutdown", "error", result)
		}
		if logFile != nil {
			result = errors.Join(result, logFile.Close())
		}
	}()
	if *check {
		ready := (<-loaded).(tui.Ready)
		if ready.Err != nil {
			return ready.Err
		}
		fmt.Printf("Configuration OK: %s\nSQLite OK: %s\nLog: %s\n", paths.Config, paths.Database, paths.Log)
		return nil
	}
	// Enable mouse only after async configuration is available via a Tea command.
	startup := func() tea.Msg { return <-loaded }
	model := tui.New(startup)
	p := tea.NewProgram(model, tea.WithAltScreen())
	stopHangup := handleHangup(p)
	defer stopHangup()
	final, e := p.Run()
	if e != nil {
		return e
	}
	m, ok := final.(tui.Model)
	if !ok {
		return fmt.Errorf("unexpected final application state")
	}
	if m.Err != nil {
		return m.Err
	}
	cancel()
	wg.Wait()
	if store != nil && m.IsReady() {
		saveCtx, done := context.WithTimeout(context.Background(), 6*time.Second)
		defer done()
		data, e := json.Marshal(m.Preferences())
		if e != nil {
			return e
		}
		if e = store.Put(saveCtx, "pane-widths", string(data)); e != nil {
			return fmt.Errorf("save layout: %w", e)
		}
	}
	return nil
}

// errDue makes --due usable in shell conditions without being reported as a failure.
type errDue struct{}

func (errDue) Error() string { return "items are due" }

// report implements --due and --notify. TideContacts has no background process: these
// flags exist so the operating system's own scheduler (a systemd user timer, cron or
// launchd) can ask a short-lived process what is due. Nothing is sent anywhere unless
// --notify is passed, and then only to the local desktop notification service.
func report(database string, notify bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := sqlite.Open(ctx, database)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	if notify {
		reminders, err := store.Reminders(ctx, true)
		if err != nil {
			return err
		}
		sent := map[string]string{}
		if raw, _ := store.Get(ctx, "notified"); raw != "" {
			_ = json.Unmarshal([]byte(raw), &sent)
		}
		current := map[string]string{}
		for _, r := range reminders {
			stamp := r.Due().UTC().Format(time.RFC3339)
			current[r.ID] = stamp
			if sent[r.ID] == stamp {
				continue // already announced for this occurrence
			}
			if err := desktopNotify("TideContacts reminder", r.Title); err != nil {
				return fmt.Errorf("desktop notification failed: %w", err)
			}
		}
		data, _ := json.Marshal(current)
		return store.Put(ctx, "notified", string(data))
	}
	items, err := store.Inbox(ctx)
	if err != nil {
		return err
	}
	count := 0
	for _, it := range items {
		if it.Urgency >= 2 && it.Status == "open" {
			count++
			fmt.Printf("%-20s %s — %s\n", domain.InboxKinds[it.Kind], it.Title, it.Reason)
		}
	}
	if count == 0 {
		fmt.Println("Nothing is due.")
		return nil
	}
	return errDue{}
}

func desktopNotify(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("osascript", "-e", fmt.Sprintf("display notification %q with title %q", body, title)).Run()
	case "windows":
		return fmt.Errorf("no notification helper on Windows; schedule --due instead")
	}
	return exec.Command("notify-send", "--app-name=TideContacts", title, body).Run()
}
