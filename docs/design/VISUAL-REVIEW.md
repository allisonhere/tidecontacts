# Phase 4A visual review

**Recommendation:** lead with Contemporary Instrument for Business Pulse and Human Workspace for records. Use Editorial Precision for period reviews and print-like reports, not the default working surface. Keep all three compositions distinct while sharing the record model, definitions, focus behavior, and brand vocabulary.

## Run the explorations

From the repository root:

```sh
python3 -m http.server 8494 --bind 127.0.0.1 --directory docs/design/explorations
```

Open `http://127.0.0.1:8494/` and choose a view. The server only needs the fixture and static files. In any view, change **Period** or **Currency**, open a source row, and press **Esc** to return. **Ctrl/⌘+K** searches across the fictional records. Opening an out-of-filter result changes to that record's currency; backing out restores the previous scope.

Build and run the actual Qt counterpart using the [native build and capture guide](explorations/contemporary-instrument/desktop/README.md). It is not a WebView. For the existing terminal Command Center, launch a compatible host and Professional extension on an isolated demo profile and choose `:` → **Open Business**. The terminal frames come from the paired regression with its standard fictional dataset; the native and browser explorations share the smaller design [fixture](explorations/fixture.json), so their amounts are not expected to match the terminal frames.

## Original captures

| Surface | Evidence |
| --- | --- |
| Editorial Precision · Chromium | [1440 × 900, opened a booked source](screenshots/web/editorial-1440x900.png) |
| Contemporary Instrument · Chromium | [1440 × 900, November USD and action queue](screenshots/web/instrument-1440x900.png) · [cross-currency command search](screenshots/web/instrument-palette.png) |
| Human Workspace · Chromium | [1440 × 900, selected relationship](screenshots/web/human-1440x900.png) · [390 × 844, attention list](screenshots/web/human-390x844.png) · [390 × 844, opened dossier](screenshots/web/human-dossier-390x844.png) |
| Contemporary Instrument · Qt Quick | [1440 × 960, native scene](screenshots/desktop/instrument-1440x960.png); desktop compositor decorations are not part of the offscreen capture. |
| Existing terminal · paired host and extension | [80 × 24](screenshots/terminal/command-80x24.txt) · [110 × 32](screenshots/terminal/command-110x32.txt) · [160 × 45](screenshots/terminal/command-160x45.txt). Actual PTY text frames, not mockups. |

## Comparative review

| View | What the screenshots demonstrate | Where it loses |
| --- | --- | --- |
| Editorial Precision | The serif edition title and paper surface are distinctive; the tabular ledger and source margin make the amount and its origin adjacent. The result uses the quiet type hierarchy admired in Things and Bear, but as a financial report, not a task or writing app. | The edition title intentionally takes vertical space. It is slower to scan than Instrument when the user needs to work several commitments rather than understand a period. |
| Contemporary Instrument | Zero-aligned bars with exact values, thin analytical rules, direct period/currency controls, and an overdue queue make the next decision legible. The browser favors a wide plot plus action queue; the Qt split keeps the source in sight while the queue and plot share the main surface. This borrows hierarchy from IBM/Helios and window composition from Nova, not their colors. | At 900 px high the web inspector can extend below the fold. Keep the queue-to-source focus handoff; for wide production workspaces, test the native side inspector as an alternative. The Qt preview sets a 1080 px minimum width rather than crushing the ledger into a phone layout. |
| Human Workspace | Identity, last touch, dated next commitment and chronology read as a dossier. The visible marker and text give overdue status without assigning a health score. Attio's record continuity and Bear's reading surface are useful analogues; the warm canvas, commitment order and explicit date reasoning are Tide's. | A full-width roster does not compare money as efficiently as a ledger, and repeated source amounts are secondary. Keep it for relationship work rather than stretching it into the revenue home. |

## Refinement from rendered evidence

1. **Hierarchy and density.** The first web frames put too much vertical space into the report titles and showed quantities on the plot only through the axis and accessible names. Reduced the mastheads, tightened queue rows, and put exact amounts directly under the month labels. Selected records still carry text/focus treatment in addition to a background accent.
2. **Navigation and encoding.** The first narrow relationship view left the full queue above a selected record; the revised view opens the dossier on its own, and Escape restores the triggering row. In native Qt, the visual capture exposed bars drawn below the zero guide by the control style's padding. Zeroed that padding, added a scene-coordinate assertion, and replaced the numbered navigation controls with Plot, Queue, and Source labels.

## Verification and limits

- **Chromium:** all three loaded and rendered at 1440 × 900 and 390 × 844 without page-level horizontal overflow. Exercised source activation and return focus, period and currency changes, search focus, Escape cancellation, and cross-currency selection/return. Instrument bars selected the booked month and exposed all five exact amounts. The narrow Human dossier replaced the list and restored row focus on Escape. Preview resources were served from loopback only.
- **Financial check:** independently recomputed November booked, open pipeline, and weighted pipeline from the fixture. USD was **28,400 / 87,300 / 42,855**; EUR was **9,200 / 19,600 / 11,320**. The EUR October comparison uses a real zero in this complete fictional window; it is not a missing-data convention. Web and native calculations round each weighted record before summing.
- **Qt Quick:** the offscreen window test exercises pointer selection, inspector keyboard focus/return, the period selector, booked-source scope, chart zero alignment, and bounds at 1200 × 780. The captured scene uses the same component graph as the interactive window.
- **Terminal:** the paired Phase 3C smoke passed keyboard/resize, filters, entitlement denial, backup/restore and disabled recovery. No terminal surface code was changed for this design study.

Measured foreground contrast against each browser canvas for the design tokens below. This is a palette check, not an exhaustive accessibility audit.

| View | Main text | Secondary text | Alert text | Focus outline |
| --- | ---: | ---: | ---: | ---: |
| Editorial Precision | 12.21:1 | 5.80:1 | 6.17:1 | 5.66:1 |
| Contemporary Instrument | 14.01:1 | 7.88:1 | 9.21:1 | 10.43:1 |
| Human Workspace | 11.83:1 | 6.23:1 | 6.51:1 | 7.03:1 |

These tests demonstrate the prototype interactions, not a production accessibility sign-off or a subjective claim of parity with the reference products. Before a graphical release, test with screen readers, larger fonts, high contrast and platform keyboards; compare task completion with users; and connect to the shared authenticated service rather than the fixture. No distribution or licensing decision is implied.

## Application captures (Phase 4C)

These show the implemented clients on the fictional demo profile, not the explorations.

| Surface | Capture |
| --- | --- |
| Browser, People with an open dossier, light, 1440 × 900 | [app-people](screenshots/web/app-people-1440x900.png) |
| Browser, Professional Business (Pulse), light, 1440 × 900 | [app-business](screenshots/web/app-business-1440x900.png) |
| Browser, Professional Automation, light, 1440 × 900 | [app-automation](screenshots/web/app-automation-1440x900.png) |
| Browser, Professional Sales, dark, 1440 × 900 | [app-sales-dark](screenshots/web/app-sales-dark-1440x900.png) |
| Browser, Tasks list and a task, dark, 390 × 844 | [list](screenshots/web/app-tasks-dark-390x844.png), [detail](screenshots/web/app-task-detail-dark-390x844.png) |
| Qt Quick desktop, People with an open dossier, 1180 × 800 | [app-desktop-people](screenshots/desktop/app-desktop-people-1180x800.png) |

Limits: no contrast or screen-reader audit was run on the application, only on the explorations. The desktop client does not follow the dark theme.
