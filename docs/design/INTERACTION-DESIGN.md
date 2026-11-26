# Phase 4A interaction contract

## Location, filters and return

- **Location** answers which workspace and view is visible. Keep it stable while a record inspector opens. Narrow clients replace the list with the selected record instead of shrinking three panes until unreadable. Returning restores the entry point, selection, scroll and filters. A record no longer in the result set gets an honest nearest-row or empty-state fallback, not an unexpected filter reset.
- **Scope** answers which data this view uses. Show the selected currency, booked period and as-of date above the result. Changing period filters booked transactions; the *current* pipeline and attention queue remain an as-of snapshot, not reconstructed history. Changing currency also changes available records; never substitute one currency into a stale selection. State when a scope has no records.
- **Evidence** answers why a number or signal appears. A booked month opens the corresponding won records, not all expected closes. Open value sums open records in that currency; weighted value is the sum of each amount × probability in basis points, rounded half-up per record. State that forecasts are not booked sales or cash. Commitment labels are date comparisons against the fixture date. No predictive or inferred relationship scores.

## Input and focus

| Action | Web and desktop intent | Terminal baseline |
| --- | --- | --- |
| Move | Use native focus order; arrows may move within a dedicated selector. Focus is distinguishable from the selected row and not color-only. | `Tab` changes pane, arrows or `j` / `k` select. |
| Inspect | Activate a real labelled control with Enter/Space or click. An explicit drill-down opens evidence without editing it. | Enter opens selected evidence. |
| Return | Escape closes the active search/dialog or the narrow inspector first; focus returns to its trigger. On wide screens selection can stay visible while focus returns to the source list. | Esc restores parent view. |
| Search and commands | Ctrl/⌘+K is additive to a visible affordance. Scope commands to the current workspace and retain native platform/text-input keys. | `:` opens the palette; `/` searches supported lists. |
| Edit | Future live clients use current revision and the existing API's validation and permissions. Consequential changes must show their exact target and effect before confirmation. | Existing TUI editors remain authoritative. |

## Accessibility acceptance

Use parity between pointer and keyboard for core tasks, clear label associations, semantic headings/list/table structure, and name/role/state for custom focusable controls. Treat visible text, keys, and zero-based scales as redundant chart cues; provide exact comparable values and source records. Use a single accent and a textual marker for warning/overdue. Verify keyboard order, dialog exit, no sideways page scroll at narrow widths, and text clipping at larger system fonts before product sign-off. Respect reduced motion; no transition is required to understand state.

## What the fixture does not simulate

The previews have no sign-in, persistence, synchronization, write permissions or approval buttons. They support navigation and analysis over deterministic fictional values only. They are not a substitute for testing screen readers, keyboard layouts, high-contrast settings and window systems in each future production client.
