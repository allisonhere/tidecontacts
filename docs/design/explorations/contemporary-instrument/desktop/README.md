# Contemporary Instrument · Qt Quick

A read-only TideContacts revenue desk. The C++ model reads the shared fictional fixture and calculates the report; Qt Quick draws the desktop and handles focus. No browser, database, network service, or installation is involved.

## Build and open

From the repository root, with Qt 6 Quick, Quick Controls, Test, `qmake6`, and a C++ toolchain installed:

```sh
prototype="$PWD/docs/design/explorations/contemporary-instrument/desktop"
fixture="$PWD/docs/design/explorations/fixture.json"
build="$(mktemp -d)"
(cd "$build" && qmake6 "$prototype/instrument.pro" && make -j2)
"$build/tide-instrument" --fixture "$fixture"
```

For a native 1440 × 960 screenshot and the fixture verification, run in the same shell:

```sh
QT_QPA_PLATFORM=offscreen QT_QUICK_BACKEND=software QT_SCALE_FACTOR=1 \
  "$build/tide-instrument" --fixture "$fixture" \
  --capture "$build/contemporary-instrument.png" --verify
```

`--capture` saves the initial Qt scene before `--verify` changes the window. Either option also works alone. Verification checks actual pointer and key input, exact USD and EUR report values, zero alignment, the booked-source scope, and the layout at 1200 × 780.

## Review workflow

- Choose USD or EUR. Select a month on the chart to change the booked period. The two open totals cover all expected dates in the selected currency.
- Select a ledger row to read its source. **Alt+2** focuses the selected row; **↑/↓** selects the adjacent row. **Alt+3** opens keyboard focus in the inspector; **Esc** returns to the selected row. The same controls work with a pointer, and **Tab / Space** moves between and activates controls.
- **Booked sources** shows won records closed in the selected month and currency. **Open queue** returns to open records in follow-up order. **Alt+1** focuses the currency selector, **?** lists shortcuts, and **Q** closes the window.

Booked value sums won records by close date; it is not recognized revenue. Weighted forecast sums individual open values multiplied by their entered probability and rounded half up to the nearest minor unit. Open pipeline is unweighted. Currencies are never combined.
