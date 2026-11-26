# TideContacts · Phase 4A design direction

**A field journal with an instrument panel.** TideContacts should feel like a local professional's working surface: restrained, navigable, and clear about what the evidence does and does not say. The unit of design is a decision with a source record, not a card.

## Design position

- **Location, view, action.** Keep workspace location steady; put period, currency, and record scope next to the content they affect. Move commands to the record or metric that owns them.
- **Quiet structure.** Prefer alignment, font weight, and thin rules to nested containers. Ordinary controls stay compact. Reserve larger type for a record name, the report title, or a primary amount, not all three competing at once.
- **Evidence at the point of decision.** Use tabular numerals and source-record drill-down. Separate booked value, weighted forecast and open pipeline. A bright state still needs an explicit label, date, or amount. No inferred risk scores, currency aggregation, or claims about accounting revenue.
- **Tide identity.** Use a drafted T mark, ocean-ink foregrounds, pale paper, a mineral teal selection accent and a distinct ochre for time-sensitive work. The visual motif is a calibrated baseline and a thin marker, not waves, branded illustrations, neon gradients or borrowed color tokens. Fonts come from the platform, not a remote CDN.
- **Composition changes with the job.** A period-led financial view can be editorial; an operational review can be chart-and-queue; a person dossier needs contextual reading room. Do not make them cosmetically different skins of the same grid.

## Three hypotheses on the same fictional records

| Exploration | First question | Composition and visual vocabulary |
| --- | --- | --- |
| Editorial Precision | How did this month perform, and which records explain it? | Paper ledger, serif masthead, compact edition metadata, exact numbers and a record note in the margin. |
| Contemporary Instrument | What moved, and what should I review next? | Flat ocean-ink canvas, tabular metrics, explicit baseline, one plot and a queue/inspector. Test compact desktop window chrome and keyboard focus. |
| Human Workspace | Which relationship needs my attention, and why? | Warm, readable relationship dossier, three-pane orientation at width, dated commitments and quiet facts. Revenue is secondary to the person. |

Test all three with the same fixture and the same drill-down and return task. A view can have a different information order without changing financial definitions, accessibility semantics, or record permissions.

## Scope boundary

These are runnable **design explorations**, not new production clients. The browser and Qt Quick previews are local-only, use the included fictional fixture, and do not open SQLite or send network requests to TideContacts. The real terminal remains the regression reference. This phase does not establish a public API, grant integrations new access, or change Community/Professional licensing. Any live client starts with the existing service-boundary handoff and an explicit authenticated read-only adapter before permitting edits.

Review the [visual evidence](REFERENCES.md), [interaction contract](INTERACTION-DESIGN.md), and [anti-patterns](ANTI-PATTERNS.md). The [visual review](VISUAL-REVIEW.md) links the runnable previews, original captures, and verified comparisons.
