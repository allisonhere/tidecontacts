"use strict";

const root = document.querySelector("#preview");
const view = document.body.dataset.view;
const viewNames = {
  editorial: "Editorial Precision",
  instrument: "Contemporary Instrument",
  human: "Human Workspace",
};
const currencies = ["USD", "EUR"];
const dayFormat = new Intl.DateTimeFormat("en-US", {day: "numeric", month: "short", timeZone: "UTC"});
const fullDayFormat = new Intl.DateTimeFormat("en-US", {day: "numeric", month: "long", year: "numeric", timeZone: "UTC"});
const monthFormat = new Intl.DateTimeFormat("en-US", {month: "long", timeZone: "UTC"});
const periodFormat = new Intl.DateTimeFormat("en-US", {month: "long", year: "numeric", timeZone: "UTC"});
const formatters = Object.fromEntries(currencies.map(currency => [currency, new Intl.NumberFormat("en-US", {
  style: "currency", currency, minimumFractionDigits: 0, maximumFractionDigits: 2,
})]));
const chars = {"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"};
const escape = value => String(value).replace(/[&<>"']/g, char => chars[char]);
const date = iso => new Date(`${iso}T00:00:00Z`);
const day = iso => dayFormat.format(date(iso));
const fullDay = iso => fullDayFormat.format(date(iso));
const monthName = month => monthFormat.format(date(`${month}-01`));
const periodName = month => periodFormat.format(date(`${month}-01`));
const money = (minor, currency) => formatters[currency].format(minor / 100);
function percent(part, whole, approximation = "≈") {
  const scaled = part * 10000;
  return `${scaled % whole === 0 ? "" : approximation}${Math.round(scaled / whole) / 100}%`;
}

const weighted = records => records.reduce((sum, record) => sum + Math.floor((record.minor * record.probability_bp + 5000) / 10000), 0);
const total = records => records.reduce((sum, record) => sum + record.minor, 0);
const actionOrder = (a, b) => a.due.localeCompare(b.due);
const dateMarkup = (iso, label = day(iso)) => `<time datetime="${iso}">${label}</time>`;
const state = {period: "", currency: "USD", selection: null};
let fixture;
let records;
let dialog;
let search;
let searchReturn;
let commandMatches = [];

function scoped() {
  const {currency, period} = state;
  const matching = fixture.records.filter(record => record.currency === currency);
  const open = matching.filter(record => record.status === "open").sort(actionOrder);
  const won = matching.filter(record => record.status === "won");
  const booked = won.filter(record => record.closed.startsWith(period)).sort((a, b) => a.closed.localeCompare(b.closed));
  const previous = fixture.months[fixture.months.indexOf(period) - 1];
  return {
    currency, period, open, won, booked, previous,
    bookedValue: total(booked), openValue: total(open), weightedValue: weighted(open),
    previousValue: previous ? total(won.filter(record => record.closed.startsWith(previous))) : null,
    target: fixture.targets[period]?.[currency],
  };
}

function comparison(scope) {
  if (scope.previousValue === null) return "First period in this fixture";
  const delta = scope.bookedValue - scope.previousValue;
  const sign = delta > 0 ? "+" : delta < 0 ? "−" : "";
  return `${sign}${money(Math.abs(delta), scope.currency)} vs ${monthName(scope.previous)}`;
}

function dueStatus(record) {
  if (record.due < fixture.as_of) return "Overdue";
  if (record.due === fixture.as_of) return "Due today";
  return "Upcoming";
}

function dueMarkup(record) {
  const status = dueStatus(record);
  return `<span class="due ${status === "Upcoming" ? "" : "due-attention"}">${status}</span>`;
}

function shell() {
  return `
    <a class="skip-link" href="#main">Skip to content</a>
    <header class="app-bar">
      <div class="app-bar-inner">
        <a class="brand" href="../" aria-label="TideContacts explorations"><span class="tide-mark" aria-hidden="true"></span>TideContacts</a>
        <span class="app-context">${viewNames[view]}</span>
        <div class="app-tools">
          <span class="sandbox"><span aria-hidden="true"></span>Fictional workspace</span>
          <button id="command-open" class="command-open" type="button" aria-label="Search records" aria-keyshortcuts="Control+k Meta+k"><span class="search-mark" aria-hidden="true"></span><span>Search<span class="command-extra"> records</span></span><kbd>Ctrl K</kbd></button>
        </div>
      </div>
    </header>
    <main id="main" tabindex="-1"></main>
    <dialog id="record-search" aria-labelledby="search-heading">
      <div class="command-head"><h2 id="search-heading">Find a record</h2><button type="button" id="command-close" class="quiet-button">Close <kbd>Esc</kbd></button></div>
      <label class="sr-only" for="search-query">Search by contact, organization, or record</label>
      <input id="search-query" type="search" autocomplete="off" placeholder="Contact, organization, or record…" autofocus>
      <p id="result-count" class="command-caption" role="status"></p>
      <ul id="search-results" class="command-results"></ul>
      <p class="command-hint"><kbd>↓</kbd> <kbd>↑</kbd> Move <kbd>Enter</kbd> Open <kbd>Esc</kbd> Close</p>
    </dialog>`;
}

function controls(scope) {
  return `<div class="scope-controls">
    <div class="scope-field"><label for="period">Period</label><select id="period" name="period">${[...fixture.months].reverse().map(month => `<option value="${month}"${month === scope.period ? " selected" : ""}>${periodName(month)}</option>`).join("")}</select></div>
    <div class="scope-field"><label for="currency">Currency</label><select id="currency" name="currency">${currencies.map(currency => `<option value="${currency}"${currency === scope.currency ? " selected" : ""}>${currency}</option>`).join("")}</select></div>
  </div>`;
}

function recordButton(record, location, content, current = state.selection?.id === record.id) {
  return `<button type="button" id="${location}-${escape(record.id)}" class="record-button" data-record="${escape(record.id)}" aria-controls="${view === "human" ? "dossier" : "record-detail"}"${current ? ' aria-current="true"' : ""}>${content}</button>`;
}

function backButton() {
  return '<button type="button" class="quiet-button back-button" data-back><span aria-hidden="true">←</span> Back to results</button>';
}

function bookedTable(scope, location) {
  if (!scope.booked.length) return `<p class="empty">No ${scope.currency} records were booked in ${periodName(scope.period)}.</p>`;
  return `<table class="ledger-table">
    <thead><tr><th scope="col">Source record</th><th scope="col">Closed</th><th scope="col">Booked</th></tr></thead>
    <tbody>${scope.booked.map(record => `<tr data-record-row="${location}-${escape(record.id)}"><th scope="row">${recordButton(record, location, `<strong>${escape(record.organization)}</strong><small>${escape(record.title)}</small>`)}</th><td>${dateMarkup(record.closed)}</td><td>${money(record.minor, scope.currency)}</td></tr>`).join("")}</tbody>
  </table>`;
}

function selectedDetail(record) {
  const open = record.status === "open";
  return `${backButton()}
    <h2 id="record-title" tabindex="-1">${escape(record.organization)}</h2>
    <p class="record-subtitle">${escape(record.title)} · ${escape(record.contact)}</p>
    <p class="record-note">${escape(record.detail)}</p>
    <dl class="detail-facts">
      <div><dt>${open ? "Stage" : "Status"}</dt><dd>${escape(record.stage)}</dd></div>
      <div><dt>${open ? "Open value" : "Booked value"}</dt><dd>${money(record.minor, record.currency)} ${record.currency}</dd></div>
      ${open ? `<div><dt>Weighted value</dt><dd>${money(weighted([record]), record.currency)} <small>at ${record.probability_bp / 100}%</small></dd></div>
      <div><dt>Last touch</dt><dd>${dateMarkup(record.last_touch)}</dd></div>
      <div><dt>Next action due</dt><dd>${dateMarkup(record.due)} · ${dueMarkup(record)}</dd></div>` : `<div><dt>Closed</dt><dd>${dateMarkup(record.closed, fullDay(record.closed))}</dd></div>`}
    </dl>
    ${open ? `<p class="record-action">${escape(record.next)}</p>` : ""}`;
}

function openList(scope, location) {
  return `<ul class="open-list">${[...scope.open].sort((a, b) => a.expected.localeCompare(b.expected)).map(record => `<li>${recordButton(record, location, `<span><strong>${escape(record.organization)}</strong><small>${escape(record.title)} · ${escape(record.stage)}</small></span><span>${money(record.minor, scope.currency)} <span aria-hidden="true">↗</span></span>`)}</li>`).join("")}</ul>`;
}

function editorial(scope, selected) {
  const target = scope.target;
  return `<div class="editorial-page">
    <div class="edition-line"><span>Field reports <b>/</b> Financial ledger</span><span>Evidence through ${dateMarkup(fixture.as_of, fullDay(fixture.as_of))}</span></div>
    <header class="edition-header">
      <div><p class="eyebrow">The monthly edition <span aria-hidden="true">—</span> ${scope.period.slice(0, 4)}</p><h1>The ${monthName(scope.period)}<br> ledger<span>.</span></h1></div>
      <div class="edition-scope"><p>What was booked, and which records explain it?</p>${controls(scope)}</div>
    </header>
    <div class="newspaper">
      <div class="ledger-column">
        <section aria-labelledby="ledger-heading">
          <div class="journal-heading"><h2 id="ledger-heading">Booked ledger</h2><span>${periodName(scope.period)} · ${scope.currency}</span></div>
          <div class="ledger-totals">
            <div><span>Booked</span><strong>${money(scope.bookedValue, scope.currency)}</strong></div>
            <div><span>Period target</span><strong>${target ? money(target, scope.currency) : "Not set"}</strong></div>
            <div><span>Target attained</span><strong>${target ? percent(scope.bookedValue, target) : "—"}</strong></div>
          </div>
          ${target ? `<div class="attainment" role="img" aria-label="${percent(scope.bookedValue, target, "Approximately ")} of target booked"><span style="--attained:${Math.min(100, 100 * scope.bookedValue / target)}%"></span></div>` : ""}
          <p class="period-comparison">${comparison(scope)}</p>
          ${bookedTable(scope, "ledger")}
        </section>
        <section class="editorial-pipeline" aria-labelledby="pipeline-heading">
          <div class="journal-heading"><h2 id="pipeline-heading">Open pipeline</h2><span>As of ${dateMarkup(fixture.as_of)} · ${scope.currency}</span></div>
          <p class="pipeline-summary"><strong>${money(scope.openValue, scope.currency)}</strong> open <span aria-hidden="true">/</span> <strong>${money(scope.weightedValue, scope.currency)}</strong> weighted</p>
          ${openList(scope, "pipeline")}
        </section>
      </div>
      <aside id="record-detail" class="margin-note" aria-labelledby="${selected ? "record-title" : "margin-title"}">
        <p class="eyebrow note-index">${selected ? "02 / Record detail" : "01 / Reading the ledger"}</p>
        ${selected ? selectedDetail(selected) : `<h2 id="margin-title">Notes on the numbers</h2><p>Booked value comes from won records closed during the selected month. It is not accounting revenue.</p><p>Weighted pipeline uses each open record’s stated probability. It is a snapshot, not booked sales or cash.</p><p class="margin-instruction"><span aria-hidden="true">↖</span> Open a source record to read it in this margin.</p>`}
      </aside>
    </div>
    <footer class="edition-footer"><span>TideContacts · Financial ledger</span><span>Design exploration · Fictional records only</span></footer>
  </div>`;
}

function instrumentPlot(scope) {
  const series = fixture.months.map(month => ({month, value: total(scope.won.filter(record => record.closed.startsWith(month)))}));
  const largest = Math.max(...series.map(point => point.value));
  const step = largest < 1200000 ? 250000 : 1000000;
  const divisions = Math.max(1, Math.ceil(largest / step));
  const ceiling = divisions * step;
  const ticks = Array.from({length: divisions + 1}, (_, i) => `<div class="plot-tick" style="--position:${100 - i / divisions * 100}%"><span>${money(i * step, scope.currency)}</span></div>`).join("");
  return `<div class="plot" role="group" aria-label="Booked value by month. Vertical scale starts at zero. Select a bar to change the period.">
    <div class="plot-space"><div class="plot-ticks" aria-hidden="true">${ticks}</div><div class="plot-columns">
      ${series.map(point => `<button type="button" class="plot-period" id="plot-${point.month}" data-period="${point.month}" aria-pressed="${point.month === scope.period}" aria-label="${periodName(point.month)}: ${money(point.value, scope.currency)} ${scope.currency} booked" style="--height:${point.value / ceiling * 100}%"><span class="plot-bar"></span><span class="plot-label" aria-hidden="true"><b>${monthName(point.month).slice(0, 3)}</b><span class="plot-value">${money(point.value, scope.currency)}</span></span></button>`).join("")}
    </div></div>
    <div class="plot-foot"><span><i aria-hidden="true"></i> Selected period</span><span>Closed-won records only · ${scope.currency}</span></div>
  </div>`;
}

function instrument(scope, selected) {
  return `<div class="instrument-page">
    <header class="instrument-header"><div><p class="eyebrow"><span aria-hidden="true">01</span> Operations / Financial review</p><h1>Pipeline review</h1><p>Booked performance and the actions behind open work.</p></div>${controls(scope)}</header>
    <section class="instrument-metrics" aria-label="Financial summary">
      <div><span>Booked this period</span><strong>${money(scope.bookedValue, scope.currency)}</strong><small>${comparison(scope)}${scope.target ? ` · ${percent(scope.bookedValue, scope.target)} of ${money(scope.target, scope.currency)} target` : " · No target set"}</small></div>
      <div><span>Open pipeline</span><strong>${money(scope.openValue, scope.currency)}</strong><small>${scope.open.length} open records · As of ${dateMarkup(fixture.as_of)}</small></div>
      <div><span>Weighted forecast</span><strong>${money(scope.weightedValue, scope.currency)}</strong><small>Open value × stated probability · Not booked sales or cash</small></div>
    </section>
    <div class="instrument-grid">
      <div class="instrument-analysis">
        <section aria-labelledby="plot-heading">
          <div class="instrument-section-heading"><div><p class="eyebrow">Performance / 01</p><h2 id="plot-heading">Booked by month</h2></div><span>${scope.currency}</span></div>
          ${instrumentPlot(scope)}
        </section>
        <section class="instrument-sources" aria-labelledby="sources-heading"><div class="instrument-section-heading"><div><p class="eyebrow">Evidence / 02</p><h2 id="sources-heading">Booked records</h2></div><span>${periodName(scope.period)}</span></div>${bookedTable(scope, "close")}</section>
      </div>
      <div class="instrument-action">
        <section aria-labelledby="queue-heading">
          <div class="instrument-section-heading"><div><p class="eyebrow">Commitments</p><h2 id="queue-heading">Action queue</h2></div><span>${scope.open.length} open</span></div>
          <ul class="instrument-queue">${scope.open.map(record => `<li>${recordButton(record, "queue", `<span><strong>${escape(record.next)}</strong><small>${escape(record.organization)}</small></span><span class="queue-date">${dateMarkup(record.due)}${dueMarkup(record)}</span>`)}</li>`).join("")}</ul>
        </section>
        <aside id="record-detail" class="instrument-inspector" aria-labelledby="${selected ? "record-title" : "inspector-title"}">
          <p class="eyebrow">Source inspector</p>
          ${selected ? selectedDetail(selected) : `<h2 id="inspector-title">Records behind the review</h2><p>Open a commitment or booked record to inspect its value, dates, and source note.</p>`}
        </aside>
      </div>
    </div>
    <footer class="instrument-footer"><span>Sandbox / Read-only design exploration</span><span>Snapshot ${dateMarkup(fixture.as_of, fullDay(fixture.as_of))}</span></footer>
  </div>`;
}

function historyFor(record, scope) {
  const related = scope.won.filter(entry => entry.organization === record.organization && entry.contact === record.contact);
  return related.map(entry => ({when: entry.closed, html: recordButton(entry, "history", `<span><strong>${escape(entry.title)}</strong><small>Booked · ${money(entry.minor, entry.currency)}</small></span><span aria-hidden="true">↗</span>`)}));
}

function human(scope, selected) {
  const record = selected ?? scope.open[0] ?? scope.booked[0];
  return `<div class="human-page${selected ? " is-inspecting" : ""}">
    <aside class="workspace-rail" aria-label="Relationship desk sections">
      <p class="eyebrow"><span aria-hidden="true">03 /</span> Relationships</p>
      <p class="rail-title">Relationship<br> desk</p>
      <nav aria-label="On this page"><a href="#attention" class="rail-current"><span aria-hidden="true">◉</span> Attention</a><a href="#dossier"><span aria-hidden="true">◎</span> Dossier</a><a href="#financial"><span aria-hidden="true">≡</span> Financial context</a></nav>
      <div class="rail-lens"><p class="eyebrow">Review scope</p>${controls(scope)}</div>
      <p class="rail-foot">Local review<br><span>As of ${dateMarkup(fixture.as_of, fullDay(fixture.as_of))}</span></p>
    </aside>
    <section class="attention" id="attention" aria-labelledby="attention-title">
      <div class="attention-header"><p class="eyebrow">Work to follow up</p><h1 id="attention-title">Attention</h1><p>${scope.open.length} commitments · Ordered by due date</p></div>
      <ul class="attention-list">${scope.open.map(entry => `<li>${recordButton(entry, "attention", `<span class="attention-name"><strong>${escape(entry.contact)}</strong><span aria-hidden="true">↗</span></span><span class="attention-org">${escape(entry.organization)}</span><span class="attention-next">${escape(entry.next)}</span><span class="attention-due">${dueMarkup(entry)}<span aria-hidden="true">·</span>${dateMarkup(entry.due)}</span>`, entry.id === record?.id)}</li>`).join("")}</ul>
    </section>
    <section id="dossier" class="dossier" aria-labelledby="record-title">
      ${record ? dossier(record, scope, Boolean(selected)) : '<h2 id="record-title">No records in this scope</h2>'}
      <section id="financial" class="human-finance" aria-labelledby="finance-heading">
        <div class="human-section-heading"><h2 id="finance-heading">Financial context</h2><span>${periodName(scope.period)} · ${scope.currency}</span></div>
        <dl><div><dt>Booked this period</dt><dd>${money(scope.bookedValue, scope.currency)}</dd></div><div><dt>Open pipeline</dt><dd>${money(scope.openValue, scope.currency)}</dd></div><div><dt>Weighted forecast</dt><dd>${money(scope.weightedValue, scope.currency)}</dd></div></dl>
        <p>${comparison(scope)}${scope.target ? ` · ${percent(scope.bookedValue, scope.target)} of ${money(scope.target, scope.currency)} target` : " · No target set"}. Forecast is not booked sales or cash.</p>
        ${bookedTable(scope, "context")}
      </section>
    </section>
  </div>`;
}

function dossier(record, scope, selected) {
  const open = record.status === "open" ? record : scope.open.find(entry => entry.organization === record.organization && entry.contact === record.contact);
  const initials = record.contact.split(" ").map(name => name[0]).join("");
  const history = historyFor(record, scope);
  if (open) history.push({when: open.last_touch, html: `<p class="timeline-kind">Last contact</p><p>${escape(open.activity)}</p>`});
  history.sort((a, b) => b.when.localeCompare(a.when));
  return `<header class="dossier-header">
    <div class="dossier-top"><p class="eyebrow">Relationship dossier</p>${selected ? backButton() : ""}</div>
    <div class="identity"><span class="avatar" aria-hidden="true">${escape(initials)}</span><div><h2 id="record-title" tabindex="-1">${escape(record.contact)}</h2><p>${escape(record.organization)}${open?.role ? ` <span aria-hidden="true">·</span> ${escape(open.role)}` : ""}</p></div></div>
  </header>
  ${open ? `<div class="relationship-facts"><div><span>Last touch</span><strong>${dateMarkup(open.last_touch)}</strong></div><div><span>Next action due</span><strong>${dateMarkup(open.due)} <span class="touch-status">${dueMarkup(open)}</span></strong></div></div>
  <section class="next-commitment" aria-labelledby="next-heading"><p class="eyebrow" id="next-heading">Next commitment</p><h3>${escape(open.next)}</h3><p>${escape(open.detail)}</p><span>${escape(open.title)} · ${escape(open.stage)}</span></section>` : ""}
  ${record.status === "won" ? `<section class="won-note" aria-labelledby="won-heading"><p class="eyebrow">Booked source record</p><h3 id="won-heading">${escape(record.title)}</h3><p>${escape(record.detail)}</p><span>${dateMarkup(record.closed)} · ${money(record.minor, record.currency)} ${record.currency}</span></section>` : ""}
  <section class="chronology" aria-labelledby="history-heading"><div class="human-section-heading"><h2 id="history-heading">Chronology</h2><span>Recorded activity</span></div>
    <ol>${history.map(entry => `<li>${dateMarkup(entry.when)}<div>${entry.html}</div></li>`).join("")}</ol>
  </section>`;
}

const renderers = {editorial, instrument, human};

function render() {
  document.querySelector("#main").innerHTML = renderers[view](scoped(), records.get(state.selection?.id));
}

function selectRecord(id, origin, fromSearch = false) {
  const record = records.get(id);
  if (!record || (state.selection?.id === id && !fromSearch)) {
    document.querySelector("#record-title")?.focus();
    return;
  }
  state.selection = {id, origin, currency: state.currency, period: state.period, previous: state.selection};
  if (fromSearch) {
    state.currency = record.currency;
    if (record.closed) state.period = record.closed.slice(0, 7);
  }
  render();
  document.querySelector("#record-title").focus();
}

function returnToResults() {
  const old = state.selection;
  if (!old) return;
  state.selection = old.previous;
  state.currency = old.currency;
  state.period = old.period;
  render();
  (document.getElementById(old.origin) ?? document.querySelector("#command-open")).focus();
}

function chooseScope(key, value, origin) {
  if (state[key] === value) return;
  state[key] = value;
  state.selection = null;
  render();
  document.getElementById(origin)?.focus({preventScroll: true});
}

function showResults() {
  const words = search.value.toLowerCase().trim().split(/\s+/).filter(Boolean);
  commandMatches = fixture.records.filter(record => {
    const haystack = [record.organization, record.contact, record.title, record.stage, record.currency, record.next].join(" ").toLowerCase();
    return words.every(word => haystack.includes(word));
  });
  document.querySelector("#result-count").textContent = `${commandMatches.length} ${commandMatches.length === 1 ? "record" : "records"} · All periods and currencies`;
  document.querySelector("#search-results").innerHTML = commandMatches.map((record, i) => `<li><button type="button" data-match="${i}"><span><strong>${escape(record.organization)}</strong><small>${escape(record.contact)} · ${escape(record.title)}</small></span><span>${record.currency} <span aria-hidden="true">↗</span></span></button></li>`).join("") || '<li class="empty">No records match. Try a contact, organization, or title.</li>';
}

function openSearch(origin = document.activeElement) {
  if (!dialog.open) {
    searchReturn = origin;
    search.value = "";
    showResults();
    dialog.showModal();
  }
  search.focus();
}

function openMatch(index) {
  const record = commandMatches[index];
  if (!record) return;
  searchReturn = null;
  dialog.close();
  selectRecord(record.id, "command-open", true);
}

function bind() {
  dialog = document.querySelector("#record-search");
  search = document.querySelector("#search-query");
  root.addEventListener("click", event => {
    const button = event.target.closest("button");
    if (button?.id === "command-open") openSearch(button);
    else if (button?.id === "command-close") dialog.close();
    else if (button?.hasAttribute("data-back")) returnToResults();
    else if (button?.dataset.match !== undefined) openMatch(Number(button.dataset.match));
    else if (button?.dataset.period) chooseScope("period", button.dataset.period, button.id);
    else if (button?.dataset.record) selectRecord(button.dataset.record, button.id);
    else {
      const row = event.target.closest("[data-record-row]");
      if (row) document.getElementById(row.dataset.recordRow).click();
    }
  });
  root.addEventListener("change", event => {
    if (event.target.id === "period" || event.target.id === "currency") chooseScope(event.target.id, event.target.value, event.target.id);
  });
  dialog.addEventListener("close", () => { searchReturn?.focus(); searchReturn = null; });
  search.addEventListener("input", showResults);
  dialog.addEventListener("keydown", event => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      dialog.close();
      return;
    }
    const buttons = [...dialog.querySelectorAll("[data-match]")];
    const current = buttons.indexOf(document.activeElement);
    if (event.key === "Enter" && document.activeElement === search) {
      event.preventDefault();
      openMatch(0);
    } else if (buttons.length && (event.key === "ArrowDown" || event.key === "ArrowUp")) {
      event.preventDefault();
      if (event.key === "ArrowDown") buttons[Math.min(current + 1, buttons.length - 1)].focus();
      else if (current <= 0) search.focus();
      else buttons[current - 1].focus();
    }
  });
  document.addEventListener("keydown", event => {
    if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === "k") {
      event.preventDefault();
      openSearch();
    } else if (event.key === "Escape" && !dialog.open && state.selection && !(event.target instanceof HTMLSelectElement)) {
      event.preventDefault();
      returnToResults();
    }
  });
}

async function start() {
  try {
    const response = await fetch("../fixture.json");
    if (!response.ok) throw new Error(`Fixture HTTP ${response.status}`);
    fixture = await response.json();
    records = new Map(fixture.records.map(record => [record.id, record]));
    state.period = fixture.months.at(-1);
    root.innerHTML = shell();
    render();
    bind();
  } catch (error) {
    console.error(error);
    root.innerHTML = '<main class="load-message"><h1>Could not load the fictional workspace.</h1><p>Serve the explorations directory over local HTTP to load its shared fixture.</p><a href="../">Back to explorations</a></main>';
  }
}

start();
