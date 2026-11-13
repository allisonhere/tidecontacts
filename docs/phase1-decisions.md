# Phase 1 decisions

## Inspected ecosystem

Read modern-tui and its architecture, design, interaction, width, lifecycle, capability, performance, testing, visual-verification and review guidance. Read the Tide ecosystem guidance. Inspected the cached TideUI module's README, demo and actual renderer/theme/ratio APIs, WhatTheDock's UI model/view, and TideMail's model, configuration and plugin query boundary. There is no local TideUI checkout; its complete cached module is available. The selected revision is also consumed by the local Tide repository.

## Dependencies

- Go module baseline 1.26.1; development toolchain 1.27.4.
- TideUI `v0.3.2-0.20261108000219-4ef89a31ee20` (pinned existing ecosystem revision).
- Bubble Tea 1.3.10, Bubbles 1.0.0; resolved Lip Gloss version is recorded in go.mod.
- BurntSushi TOML 1.6.0 for strict configuration decoding.
- modernc SQLite 1.48.0 for portable, CGO-free local storage.
- sahilm/fuzzy 0.1.1 for scored command matching.

All dependencies are pinned by go.mod/go.sum. The restricted environment cannot reach the module proxy; cached dependencies and checksum records from Tide/TideMail were used. Transitive x/text, x/tools and x/mod versions were selected from the available compatible cache. No local module replacement is required.

## Reuse and gaps

Directly reuse TideUI Renderer, Layout (ThreeColumn, SidebarOnly, Tabbed), Pane, StatusBar, Row, SoftRow, SoftPanelOverlay, PaneRatio, theme palettes, semantic styles and contrast functions. Application state and key routing follow TideUI's view-oriented convention.

TideUI has no general single-line input or application command-palette controller. Bubbles textinput is already part of its architecture, so use that existing dependency. Palette composition uses TideUI soft rows and panels; only command matching/dispatch belongs here. No pane, scrolling engine, or theme widget is copied from a sibling. A reusable TideUI palette controller could be proposed upstream when a second consumer establishes its requirements; no upstream patch is necessary for this phase.

Ripple was not inspected or added: Bubbles supplies both simple inputs. Reconsider Ripple in Phase 3 for multiline notes, soft wrapping, and editing modes, after defining plain-text versus rich-text requirements.

## Scope and tradeoffs

Eight honest empty workspaces avoid fake data and dead creation controls. Search stores a query and reports zero results; the repository-backed search comes later. Application/domain packages have no terminal imports. No speculative plugin host or dozens of unused repository interfaces are introduced.

Persist only pane ratios, once on orderly exit. Runtime themes are session-only; TOML controls the startup theme. This keeps invalid TOML intact and avoids asynchronous save ordering hazards. SQLite migration metadata and preferences are the only schema.

Mouse is opt-in pane focus by click; it does not provide row activation, dragging, palette selection or input positioning. All actions are keyboard-accessible. Help scrolls and palettes keep the selected command visible. Tiny terminals clip through TideUI; meaningful usage is best at 40 columns or more. Reduced-color and NO_COLOR modes retain explicit focus markers. Full ASCII border substitution is deferred; TERM=dumb gets plain output.

Logs are structured JSON with configurable level; rotation is deferred. Windows/macOS paths are implemented but this milestone is exercised on Linux. No upstream publishing or release tagging is part of this implementation.

## Verification in this workspace

Passed build, gofmt checks, `go test ./...`, `go test -race ./...`, `go vet ./...`, and golangci-lint (zero issues). Snapshot frames cover 160×45, 110×32, 80×24 and 60×20; deterministic randomized rendering covers tiny sizes and overlays. The PTY smoke script initializes/reopens storage, exercises navigation, search, palette and resizing, verifies terminal modes and alternate-screen cleanup for q/Ctrl+C/SIGTERM/SIGHUP, and checks invalid configuration preservation.

This workspace exposes a restricted nonfunctional `.git` directory, so builds use `GOFLAGS=-buildvcs=false` or the Makefile. Go's cache was redirected to `/tmp` for the sandbox. The optional vulnerability audit could not fetch vuln.go.dev because network/DNS access is restricted; it is not reported as passing. A tmux visual session was also disallowed by the sandbox; deterministic frames were reviewed and actual terminal behavior exercised through owned PTYs instead.
