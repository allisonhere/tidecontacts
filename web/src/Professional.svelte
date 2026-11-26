<script lang="ts">
  import { refHref } from './api';

  type Workspace = 'sales' | 'business' | 'automation';
  type Row = Record<string, unknown>;
  type Tab = { id: string; label: string; intro: string; detail?: string };
  let { workspace, capabilities, token }: { workspace: Workspace; capabilities: string[]; token: string } = $props();

  const tabs: Record<Workspace, Tab[]> = {
    sales: [
      { id: 'opportunities', label: 'Opportunities', intro: 'Deal values, next actions and recorded stage changes.', detail: 'View deal' },
      { id: 'sales-pipelines', label: 'Sales pipelines', intro: 'Configured stages, requirements and default probabilities.', detail: 'Inspect pipeline' },
      { id: 'customers', label: 'Customers', intro: 'Recorded customer lifecycles and sales relationships.' },
      { id: 'products', label: 'Products', intro: 'Catalog prices and billing terms, not inventory or invoices.' }
    ],
    business: [
      { id: 'pulse', label: 'Pulse', intro: 'Booked sales for the period and the open pipeline now, separately by currency.' },
      { id: 'briefing', label: 'Briefing', intro: 'Today’s recorded commitments and items that warrant attention.' },
      { id: 'forecast', label: 'Forecast', intro: 'Open and probability-weighted deal value, not booked sales or cash.' },
      { id: 'rescue', label: 'Rescue', intro: 'Recorded reasons to review deals. These are not predictions of loss.' },
      { id: 'watchtower', label: 'Watchtower', intro: 'Recorded customer facts, not inferred satisfaction or risk.' },
      { id: 'concentration', label: 'Concentration', intro: 'Shares of booked sales by customer within each currency.' },
      { id: 'renewals', label: 'Renewals', intro: 'Recorded renewal and notice dates, with open values by currency.' },
      { id: 'targets', label: 'Targets', intro: 'Measured progress against recorded goals.' },
      { id: 'scenarios', label: 'Scenarios', intro: 'Saved assumptions. These are not actual sales or changes to deals.', detail: 'View scenario comparison' },
      { id: 'replay', label: 'Replay', intro: 'Compare today with the facts known at the end of a selected day.' },
      { id: 'reports', label: 'Reports', intro: 'Run a saved report against recorded data without changing its definition.', detail: 'Run saved report' }
    ],
    automation: [
      { id: 'rules', label: 'Rules', intro: 'Inspect triggers, conditions and configured actions.', detail: 'Show rule' },
      { id: 'runs', label: 'Runs', intro: 'Recent executions: review the reason, action evidence and recorded outcome.', detail: 'Open trace' },
      { id: 'approvals', label: 'Approvals', intro: 'Review recent pending action requests. Decisions are not available here.' },
      { id: 'playbooks', label: 'Playbooks', intro: 'Inspect saved steps and inputs without starting a playbook.', detail: 'Inspect playbook' },
      { id: 'schedules', label: 'Schedules', intro: 'Inspect timing and next occurrences, without running or changing a schedule.', detail: 'Inspect schedule' }
    ]
  };
  const periods = ['today', 'week', 'month', 'quarter', 'year', 'last30', 'last90', 'last_month', 'last_quarter', 'last_year'];
  const currencyOps = new Set(['opportunities', 'pulse', 'briefing', 'forecast', 'rescue', 'watchtower', 'concentration', 'renewals', 'replay']);
  const periodOps = new Set(['pulse', 'forecast', 'concentration']);
  const workspaceName = { sales: 'Sales', business: 'Business', automation: 'Automation' };

  let selection = $state({ workspace: '' as string, operation: '' });
  let available = $derived(tabs[workspace].filter(({ id }) => capabilities.includes(id)));
  let tab = $derived((selection.workspace === workspace && available.find(({ id }) => id === selection.operation)) || available[0]);
  let op = $derived(tab?.id ?? '');
  let filter = $state({ op: '', id: '', title: '', q: '', period: '', currency: '', date: '', offset: 0 });
  let current = $derived(filter.op === op ? filter : { op, id: '', title: '', q: '', period: '', currency: '', date: '', offset: 0 });
  let paged = $derived(!current.id && (op === 'opportunities' || op === 'rules'));
  let canSearch = $derived(!current.id && (op === 'opportunities' || op === 'products'));
  let canPeriod = $derived(periodOps.has(op) || (op === 'reports' && !!current.id));
  let canCurrency = $derived((currencyOps.has(op) && !current.id) || ((op === 'reports' || op === 'scenarios') && !!current.id));
  let retry = $state(0);
  let endpoint = $derived.by(() => {
    if (!op || (op === 'replay' && !current.date)) return '';
    const query = new URLSearchParams();
    if (canSearch && current.q.trim()) query.set('q', current.q.trim());
    if (canPeriod && current.period) query.set('period', current.period);
    if (canCurrency && current.currency.trim()) query.set('currency', current.currency.trim().toUpperCase());
    if (op === 'replay') query.set('date', current.date);
    if (paged && current.offset) query.set('offset', String(current.offset));
    return `/api/v1/pro/${op}${current.id ? `/${encodeURIComponent(current.id)}` : ''}${query.size ? `?${query}` : ''}`;
  });
  type Reply = { endpoint: string; token: string; retry: number; data: unknown; error?: string; more?: boolean };
  let reply = $state<Reply>();
  let result = $derived(reply?.endpoint === endpoint && reply.token === token && reply.retry === retry ? reply : undefined);
  $effect(() => {
    const url = endpoint;
    const bearer = token;
    const attempt = retry;
    if (!url) return;
    const controller = new AbortController();
    // A short delay avoids a request for every keystroke while typing filters.
    const timer = setTimeout(async () => {
      try {
        const response = await fetch(url, { headers: { Authorization: `Bearer ${bearer}`, Accept: 'application/json' }, signal: controller.signal, cache: 'no-store' });
        const body = row(await response.json());
        if (!response.ok || body.schema_version !== 1 || !Object.hasOwn(body, 'data')) {
          throw new Error(text(row(body.error).message) || `Unable to load this view (HTTP ${response.status}).`);
        }
        if (!controller.signal.aborted) reply = { endpoint: url, token: bearer, retry: attempt, data: body.data, more: row(body.meta).possibly_truncated === true };
      } catch (error) {
        if (!controller.signal.aborted) reply = { endpoint: url, token: bearer, retry: attempt, data: null, error: error instanceof Error ? error.message : 'Unable to load this view.' };
      }
    }, 150);
    return () => { clearTimeout(timer); controller.abort(); };
  });

  function select(id: string) {
    if (id === op) return;
    selection = { workspace, operation: id };
    filter = { op: id, id: '', title: '', q: '', period: '', currency: '', date: '', offset: 0 };
  }
  function change(patch: Partial<typeof filter>) { filter = { ...current, offset: 0, ...patch }; }
  function open(id: string, title: string) { filter = { ...current, id, title }; }
  function row(value: unknown): Row { return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Row : {}; }
  function list(value: unknown): unknown[] { return Array.isArray(value) ? value : []; }
  function rows(value: unknown): Row[] { return list(value).map(row); }
  function text(value: unknown): string { return typeof value === 'string' ? value : ''; }
  function label(key: string): string {
    if (key === 'booked_mrr_minor') return 'Booked monthly recurring rate';
    if (key === 'booked_arr_minor') return 'Booked annual recurring rate';
    if (key === 'mrr_minor') return 'Monthly recurring rate';
    if (key === 'arr_minor') return 'Annual recurring rate';
    if (key === 'weighted_minor') return 'Probability-weighted value';
    if (key === 'net_value_minor') return 'Net value';
    const name = key.replace(/_(?:minor|bp|tenths)$/, '').replace(/([a-z])([A-Z])/g, '$1 $2').replaceAll('_', ' ').replaceAll('-', ' ');
    return name.charAt(0).toUpperCase() + name.slice(1);
  }

  // ISO currencies whose exponent differs from common browser/CLDR defaults
  // are included explicitly. For unrecognized codes do not guess two digits.
  const digits = new Map<string, number>();
  for (const [exp, codes] of [
    [0, 'BIF CLP DJF GNF ISK JPY KMF KPW KRW PYG RWF UGX VND VUV XAF XOF XPF'],
    [2, 'AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BMD BND BOB BRL BSD BTN BWP BYN BZD CAD CDF CHF CNY COP CRC CUP CVE CZK DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GTQ GYD HKD HNL HTG HUF IDR ILS INR IRR JMD KES KGS KHR KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MYR MZN NAD NGN NIO NOK NPR NZD PAB PEN PGK PHP PKR PLN QAR RON RSD RUB SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP THB TJS TMT TOP TRY TTD TWD TZS UAH USD UYU UZS VES WST XCD YER ZAR ZMW ZWL'],
    [3, 'BHD IQD JOD KWD OMR TND'], [4, 'CLF']
  ] as const) for (const code of codes.split(' ')) digits.set(code, exp);
  function money(value: unknown, currency: string): string {
    const exact = typeof value === 'string' && /^-?\d+$/.test(value) ? value : Number.isSafeInteger(value) ? String(value) : '';
    if (!exact) return value == null ? 'Not recorded' : 'Exact amount unavailable';
    const exp = digits.get(currency);
    if (exp === undefined) return `${exact}${currency ? ` ${currency}` : ''} minor units`;
    const negative = exact.startsWith('-');
    const padded = (negative ? exact.slice(1) : exact).padStart(exp + 1, '0');
    const whole = (exp ? padded.slice(0, -exp) : padded).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    return `${currency} ${negative ? '−' : ''}${whole}${exp ? `.${padded.slice(-exp)}` : ''}`;
  }
  function decimal(value: unknown, currency: string): string { return typeof value === 'string' && /^-?\d+(?:\.\d+)?$/.test(value) ? `${currency} ${value}`.trim() : 'Not recorded'; }
  function pct(value: unknown, unit = '%'): string { return typeof value === 'number' ? `${value / 100}${unit}` : 'Not available'; }
  function scalar(value: unknown, key: string, currency: string): string {
    if (key.endsWith('_minor')) return money(value, currency);
    if (key.endsWith('_bp')) return pct(value);
    if (value == null) return 'Not recorded';
    if (typeof value === 'boolean') return value ? 'Yes' : 'No';
    if (typeof value === 'number') {
      if (!Number.isSafeInteger(value)) return 'Exact value unavailable';
      if (key.endsWith('_tenths')) return String(value / 10);
      if (key === 'elapsed_ms') return `${value} ms`;
      return String(value);
    }
    return text(value).replace(/^0001-01-01T00:00:00Z$/, 'Not recorded');
  }
  function present(value: unknown): boolean { return value !== undefined && value !== null && value !== '' && (!Array.isArray(value) || value.length > 0) && (typeof value !== 'object' || Object.keys(value).length > 0); }
  function community(kind: unknown, id: unknown): string | undefined {
    const k = text(kind);
    return refHref({ kind: k === 'contact' || k === 'people' ? 'person' : k, id: text(id) });
  }
  function identity(r: Row, operation: string): string {
    return text(r.title) || text(r.Title) || text(r.name) || text(r.rule_name) || text(row(r.target).name) || text(r.CustomerName)
      || (operation === 'approvals' && label(text(row(r.definition).kind))) || text(r.id) || text(r.ID) || 'Untitled record';
  }
  function recordId(r: Row): string { const id = text(r.id) || text(r.ID); return id.startsWith('-') || id === '.' || id === '..' ? '' : id; }
  function band(value: unknown): number { return typeof value === 'number' ? Math.max(0, Math.min(10000, value)) : 0; }
  function delta(value: unknown): string {
    const d = row(value);
    if (!d.available) return text(d.reason) || 'Comparison unavailable';
    if (d.kind === 'points') return pct(d.delta, ' percentage points');
    if (d.kind === 'days' && typeof d.delta === 'number') return `${d.delta / 10} days`;
    if (d.kind === 'count') return scalar(d.delta, '', '');
    return pct(d.change_bp);
  }
  const pulseFacts: [string, string, string?][] = [
    ['Active pipeline', 'active', 'total_minor'], ['Weighted open pipeline', 'weighted_minor'], ['Booked sales in period', 'booked', 'total_minor'],
    ['Lost deal value in period', 'lost', 'total_minor'], ['Win rate', 'win_rate_bp'], ['Average booked deal', 'average_deal_minor'],
    ['Next 30 days (expected)', 'expected_next_30_days', 'total_minor'], ['Overdue close (expected)', 'expected_overdue', 'total_minor'],
    ['Booked monthly recurring rate', 'booked_mrr_minor'], ['Booked annual recurring rate', 'booked_arr_minor']
  ];
  const columns: Record<string, string[]> = {
    'sales-pipelines': ['description', 'currency', 'template', 'archived', 'revision'],
    customers: ['kind', 'lifecycle', 'suggested_lifecycle', 'open_opportunities', 'won_opportunities', 'last_won', 'has_profile'],
    products: ['description', 'sku', 'category', 'unit_price_minor', 'billing_type', 'unit', 'active', 'archived', 'tags'],
    rescue: ['customer', 'status', 'stage', 'days_in_stage', 'expected_stage_days', 'value_minor', 'level', 'signals', 'last_recorded_interaction', 'next_action', 'next_action_due', 'suggested_review'],
    watchtower: ['kind', 'lifecycle', 'level', 'open_opportunities', 'last_recorded_interaction'],
    scenarios: ['description', 'horizon', 'revision'],
    reports: ['dataset', 'group_by', 'period', 'date_field', 'display', 'preset', 'filter'],
    rules: ['description', 'category', 'enabled', 'archived', 'auto_authorized', 'policy', 'revision', 'expires_on', 'cooldown_days', 'max_executions'],
    runs: ['created_at', 'policy', 'rule_revision', 'record_revision', 'sequence'],
    approvals: ['status', 'code', 'expires_at', 'snoozed_until', 'updated_at', 'attempts'],
    playbooks: ['description', 'category', 'entities', 'bundled', 'revision'],
    schedules: ['paused', 'missed_policy', 'catch_up_limit', 'next_at', 'pending_at', 'rule_revision', 'revision']
  };
</script>

{#snippet reference(kind: unknown, id: unknown, name: unknown = id)}
  {@const url = community(kind, id)}
  {#if url}<a href={url}>{text(name) || text(id)}</a>{:else if text(id) || text(name)}<span>{text(name) || text(id)}</span>{/if}
{/snippet}

{#snippet facts(record: Row, keys: string[], currency = text(record.currency))}
  {#if keys.some((key) => present(record[key]))}
    <dl class="facts">
      {#each keys as key}
        {#if present(record[key])}
          <div><dt>{label(key)}</dt><dd>{#if Array.isArray(record[key])}{list(record[key]).filter((v) => typeof v === 'string' || typeof v === 'number').join(', ')}{:else}{scalar(record[key], key, currency)}{/if}</dd></div>
        {/if}
      {/each}
    </dl>
  {/if}
{/snippet}

{#snippet evidence(record: Row, keys: string[], inherited = '')}
  {@const currency = text(record.currency) || inherited}
  <dl class="facts evidence">
    {#each keys as key}
      {@const value = record[key]}
      {#if present(value)}
        <div><dt>{label(key)}</dt><dd>
          {#if key === 'number' && currency}{money(value, currency)}
          {:else if Array.isArray(value)}
            <ul class="plain">{#each value as item}<li>{#if item && typeof item === 'object'}{@render evidence(row(item), Object.keys(row(item)), currency)}{:else}{scalar(item, key, currency)}{/if}</li>{/each}</ul>
          {:else if value && typeof value === 'object'}
            {@const item = row(value)}
            {#if community(item.kind, item.id)}{@render reference(item.kind, item.id, item.name)}{:else}{@render evidence(item, Object.keys(item), currency)}{/if}
          {:else}{scalar(value, key, currency)}{/if}
        </dd></div>
      {/if}
    {/each}
  </dl>
{/snippet}

{#snippet ids(kind: string, values: unknown)}
  {#if list(values).length}<ul class="plain links">{#each list(values) as id}<li>{@render reference(kind, id)}</li>{/each}</ul>{/if}
{/snippet}

{#snippet related(r: Row)}
  {#if text(r.record_id) && community(r.record_kind, r.record_id)}<p>Source · {@render reference(r.record_kind, r.record_id)}</p>{/if}
  {#if text(r.task_id) && r.task_id !== r.record_id}<p>Task · {@render reference('task', r.task_id)}</p>{/if}
  {#if text(r.person_id) && r.person_id !== r.record_id}<p>Person or organization · {@render reference(r.person_kind, r.person_id)}</p>{/if}
{/snippet}

{#snippet notes(value: unknown)}
  {#if list(value).length}<ul class="notes">{#each list(value) as note}<li>{text(note)}</li>{/each}</ul>{/if}
{/snippet}

{#snippet opportunity(r: Row, full: boolean)}
  {@const c = text(r.currency)}
  {@const org = row(r.organization)}
  {@const person = row(r.primary_contact)}
  <dl class="facts deal-facts">
    <div><dt>Status</dt><dd>{label(text(r.status)) || 'Not recorded'}</dd></div>
    <div><dt>Pipeline / stage</dt><dd>{text(row(r.pipeline).name) || 'Unknown pipeline'} / {text(row(r.stage).name) || 'Unknown stage'}</dd></div>
    <div><dt>Estimated deal value</dt><dd class="figure">{present(r.amount) ? decimal(r.amount, c) : money(r.amount_minor, c)}</dd></div>
    {#if present(r.final_amount_minor)}<div><dt>Value at closing</dt><dd>{money(r.final_amount_minor, c)}</dd></div>{/if}
    {#if present(r.expected_close)}<div><dt>Expected close</dt><dd>{text(r.expected_close)}</dd></div>{/if}
    {#if present(r.actual_close)}<div><dt>Actual close</dt><dd>{text(r.actual_close)}</dd></div>{/if}
    {#if text(org.id)}<div><dt>Organization</dt><dd>{@render reference('organization', org.id, org.name)}</dd></div>{/if}
    {#if text(person.id)}<div><dt>Primary contact</dt><dd>{@render reference('person', person.id, person.name)}</dd></div>{/if}
    {#if present(r.next_action)}<div><dt>Next action</dt><dd>{text(r.next_action)} {text(r.next_action_due)}</dd></div>{/if}
  </dl>
  {#if full}
    {@render facts(r, ['description', 'notes', 'owner', 'lead_source', 'priority', 'amount_basis', 'net_value_minor', 'weighted_value_minor', 'probability_bp', 'probability_source', 'discount_minor', 'estimated_cost_minor', 'one_time_revenue_minor', 'monthly_recurring_revenue_minor', 'annual_recurring_revenue_minor', 'total_contract_value_minor', 'contract_months', 'billing_frequency', 'stage_age_days', 'stage_entered_at', 'proposal_sent', 'won_reason', 'lost_reason', 'hold_reason', 'tags', 'competitors', 'archived', 'revision'])}
    {#if rows(r.contacts).length}
      <h4>Contact roles</h4><ul class="plain">{#each rows(r.contacts) as person}<li>{@render reference('person', person.contact_id, person.name)} · {list(person.roles).map((role) => label(text(role))).join(', ')}</li>{/each}</ul>
    {/if}
    {#if list(r.tasks).length}<h4>Linked tasks</h4>{@render ids('task', r.tasks)}{/if}
    {#if list(r.projects).length}<h4>Linked projects</h4>{@render ids('project', r.projects)}{/if}
    {#if list(r.activities).length}<h4>Linked activities</h4>{@render ids('activity', r.activities)}{/if}
    {#if rows(r.line_items).length}
      <h4>Line items</h4><ol class="plain ledger">{#each rows(r.line_items) as item}<li><strong>{text(item.description) || 'Line item'}</strong>{@render facts(item, ['quantity', 'unit_price_minor', 'discount', 'billing_type', 'extended_minor'], c)}</li>{/each}</ol>
    {/if}
  {/if}
{/snippet}

{#snippet genericList(items: Row[], operation: string)}
  {#if !items.length}<p class="empty">No records in this view.</p>{:else}
    <ul class="plain ledger">
      {#each items as item}
        {@const name = identity(item, operation)}
        {@const id = recordId(item)}
        <li>
          <div class="record-head"><h3>
            {#if operation === 'customers' || operation === 'watchtower'}{@render reference(item.kind, item.entity_id ?? item.id, name)}{:else}{name}{/if}
          </h3>
          {#if tab?.detail && tab.id === operation && id}<button type="button" onclick={() => open(id, name)}>{tab.detail}</button>{/if}</div>
          {#if operation === 'opportunities'}{@render opportunity(item, false)}{:else}{@render facts(item, columns[operation] ?? [])}{/if}
          {#if operation === 'runs'}<p class="muted">Execution {text(item.id)}{#if text(row(item.record).kind)} · {label(text(row(item.record).kind))}: {@render reference(row(item.record).kind, row(item.record).id)}{/if}</p>{/if}
          {#if operation === 'rules'}{@render facts(row(item.trigger), ['kind', 'entity', 'event'])}{/if}
          {#if operation === 'schedules'}{@render facts(row(item.timing), ['kind', 'timezone', 'start_date', 'local_time', 'interval_minutes'])}{/if}
          {#if operation === 'sales-pipelines'}<p class="muted">{rows(item.stages).length} stages</p>{/if}
          {#if operation === 'watchtower'}
            {#each Object.entries(row(item.open_pipeline_minor)) as [code, amount]}<p>{money(amount, code)} open pipeline</p>{/each}
            {#if rows(item.facts).length}
              <ul class="plain trace">
                {#each rows(item.facts) as fact}<li><strong>{text(fact.text) || label(text(fact.kind))}</strong>{@render facts(fact, ['date', 'level'])}{@render related(fact)}</li>{/each}
              </ul>
            {/if}
          {/if}
          {#if operation === 'approvals'}
            {@render facts(row(item.definition), ['kind', 'text', 'field', 'task_status', 'delay_days', 'due_on'])}
            {#if capabilities.includes('runs') && text(item.execution_id) && !text(item.execution_id).startsWith('-')}
              <button type="button" onclick={() => { select('runs'); filter = { ...filter, id: text(item.execution_id), title: 'Execution trace' }; }}>Open execution trace</button>
            {/if}
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
{/snippet}

{#snippet pulse(data: Row)}
  {@const period = row(data.period)}
  {@const compared = row(data.compared_with)}
  <div class="edition">
    <p class="eyebrow">Period edition</p>
    <h3>{text(period.label) || 'Performance overview'}</h3>
    <p>{text(period.from)} to {text(period.to)}{#if text(compared.from)} · Previous: {text(compared.from)} to {text(compared.to)}{/if}</p>
    {@render facts(data, ['earliest_record', 'filter'])}
  </div>
  {#if !rows(data.currencies).length}<p class="empty">No currency figures for this scope.</p>{/if}
  {#each rows(data.currencies) as group}
    {@const c = text(group.currency)}
    <section class="currency">
      <h3>{c || 'Currency unavailable'}</h3>
      <dl class="metrics">
        {#each pulseFacts as [name, key, sub]}
          <div><dt>{name}</dt><dd>{sub ? money(row(group[key])[sub], c) : scalar(group[key], key, c)}</dd>
            {#if sub}<dd class="muted">{row(group[key]).count ?? 0} deals · {row(group[key]).unknown_value_count ?? 0} without a known value</dd>{/if}
          </div>
        {/each}
      </dl>
      {@render facts(group, ['unknown_probability_count', 'average_cycle_days_tenths', 'recurring_contracts'])}
      {#if Object.keys(row(group.changes)).length}
        <details><summary>Changes against previous period</summary><dl class="facts">
          {#each Object.entries(row(group.changes)) as [key, value]}<div><dt>{label(key)}</dt><dd>{delta(value)}</dd></div>{/each}
        </dl></details>
      {/if}
    </section>
  {/each}
{/snippet}

{#snippet forecast(data: Row)}
  {@render facts(data, ['as_of', 'closed_from', 'closed_to', 'filter'])}
  {#if !rows(data.currencies).length}<p class="empty">No forecast groups for this scope.</p>{/if}
  {#each rows(data.currencies) as group}
    {@const c = text(group.currency)}
    <section class="currency"><h3>{c || 'Currency unavailable'}</h3>
      <div class="measure-grid">
        {#each [['Active pipeline', 'active'], ['On-hold pipeline', 'on_hold'], ['Closed won in period', 'closed_won'], ['Closed lost in period', 'closed_lost'], ['Pipeline recurring rates', 'pipeline_recurring'], ['Booked recurring rates', 'booked_recurring_won']] as [title, key]}
          <section><h4>{title}</h4>{@render facts(row(group[key]), ['count', 'total_minor', 'weighted_minor', 'value_minor', 'known_value_count', 'unknown_value_count', 'unknown_probability_count', 'average_value_minor', 'average_days_to_close_tenths', 'mrr_minor', 'arr_minor', 'one_time_minor'], c)}</section>
        {/each}
      </div>
      {#each [['Expected close by month', 'expected_by_month'], ['Expected close by quarter', 'expected_by_quarter'], ['Active pipeline by stage', 'by_stage']] as [title, key]}
        {#if rows(group[key]).length}
          <details open={key === 'expected_by_month'}><summary>{title}</summary>
            <div class="table-wrap"><table><thead><tr><th scope="col">{key === 'by_stage' ? 'Stage' : 'Period'}</th><th scope="col">Deals</th><th scope="col">Open value</th><th scope="col">Weighted</th><th scope="col">Unknown value</th></tr></thead>
            <tbody>{#each rows(group[key]) as bucket}<tr><th scope="row">{text(bucket.period) || `${text(bucket.pipeline)} / ${text(bucket.stage)}`}</th><td>{scalar(bucket.count, '', '')}</td><td>{money(bucket.total_minor, c)}</td><td>{money(bucket.weighted_minor, c)}</td><td>{scalar(bucket.unknown_value, '', '')}</td></tr>{/each}</tbody></table></div>
          </details>
        {/if}
      {/each}
      <div class="measure-grid">{#each [['Expected close overdue', 'expected_overdue'], ['Expected close undated', 'expected_undated']] as [title, key]}<section><h4>{title}</h4>{@render facts(row(group[key]), ['count', 'total_minor', 'weighted_minor', 'unknown_value'], c)}</section>{/each}</div>
    </section>
  {/each}
  {@render notes(data.notes)}
{/snippet}

{#snippet concentration(data: Row)}
  {@render facts(row(data.period), ['label', 'from', 'to'])}
  {#if !Object.keys(row(data.currencies)).length}<p class="empty">No booked sales groups for this scope.</p>{/if}
  {#each Object.entries(row(data.currencies)) as [code, value]}
    {@const group = row(value)}
    <section class="currency"><h3>{code}</h3>
      {@render facts(group, ['basis', 'total_minor', 'customers', 'top1_bp', 'top3_bp', 'top5_bp', 'threshold_bp', 'threshold_exceeded'], code)}
      <ol class="plain ledger shares">{#each rows(group.shares) as share}
        <li><div class="record-head"><strong>{@render reference(share.kind, share.key, share.label)}</strong><span>{pct(share.share_bp)} · {money(share.total_minor, code)}</span></div>
          <meter min="0" max="10000" value={band(share.share_bp)} aria-label={`${text(share.label)} share of ${code} booked sales`}></meter>
        </li>
      {/each}</ol>
    </section>
  {/each}
{/snippet}

{#snippet briefing(data: Row)}
  {@render facts(data, ['as_of', 'filter'])}
  {#each rows(data.sections) as section}
    <section class="currency"><h3>{text(section.title) || 'Briefing section'}</h3>
      {#if !rows(section.items).length}<p class="empty">No entries in this section.</p>{/if}
      <ul class="plain ledger">{#each rows(section.items) as item}<li><h4>{text(item.title)}</h4><p>{text(item.reason)}</p>
        {@render facts(item, ['date', 'level', 'source', 'amount_minor', 'suggested_action'])}{@render notes(item.facts)}{@render related(item)}
      </li>{/each}</ul>
    </section>
  {:else}<p class="empty">No briefing sections for this scope.</p>{/each}
  {@render notes(data.unavailable)}
{/snippet}

{#snippet renewals(data: unknown)}
  {@const r = row(data)}
  {@const items = rows(Array.isArray(data) ? data : r.items)}
  {@render facts(r, ['as_of', 'from', 'to'])}
  {#each [['Overdue open renewals', 'overdue'], ['Upcoming open renewals (cumulative)', 'windows']] as [title, key]}
    {#if rows(r[key]).length}<section class="currency"><h3>{title}</h3><div class="measure-grid">
      {#each rows(r[key]) as bucket}<section><h4>{text(bucket.currency)} {#if key === 'windows'}· {bucket.within_days} days{/if}</h4>{@render facts(bucket, ['total_minor', 'count', 'unknown_amount_count'])}</section>{/each}
    </div></section>{/if}
  {/each}
  {#if !items.length}<p class="empty">No recorded renewals in this scope.</p>{/if}
  <ul class="plain ledger">{#each items as item}<li>
    <h3>{identity(item, 'renewals')}</h3>
    {#if text(item.CustomerID ?? item.customer_id)}<p>Customer · {@render reference(item.CustomerKind ?? item.customer_kind, item.CustomerID ?? item.customer_id, item.CustomerName ?? item.customer_name)}</p>{/if}
    <p class="figure">{money(item.amount_minor ?? item.Amount, text(item.currency ?? item.Currency))}</p>
    {@render facts(item, ['Date', 'Notice', 'Status', 'Frequency', 'Start', 'End', 'DaysLeft', 'Overdue', 'Reminding', 'Suggested', 'Notes', 'renewal_date', 'notice_date', 'status', 'frequency', 'notes'])}
    {#if list(item.Tasks ?? item.tasks).length}<h4>Linked tasks</h4>{@render ids('task', item.Tasks ?? item.tasks)}{/if}
  </li>{/each}</ul>
{/snippet}

{#snippet targets(data: unknown)}
  {@const board = row(data)}
  {@render facts(board, ['as_of', 'pace_formula'])}
  {#each (Array.isArray(data) ? [['Saved targets', data]] : [['Current targets', board.current], ['Past targets', board.past]]) as [title, items]}
    <section class="currency"><h3>{text(title)}</h3>
      {#if !rows(items).length}<p class="empty">No targets in this section.</p>{/if}
      <ul class="plain ledger">{#each rows(items) as item}
        {@const t = item.target ? row(item.target) : item}
        {@const c = text(t.currency)}
        <li><h4>{identity(t, 'targets')}</h4>
          {@render facts(t, ['metric', 'start', 'end', 'description', 'active'])}
          <dl class="facts"><div><dt>Goal</dt><dd>{c ? money(t.goal, c) : scalar(t.goal, '', '')}</dd></div>
            {#each ['current', 'remaining', 'required_per_day', 'linear_pace_to_date'] as key}{#if present(item[key])}<div><dt>{label(key)}</dt><dd>{c ? money(item[key], c) : scalar(item[key], key, '')}</dd></div>{/if}{/each}
          </dl>
          {#if present(item.completed_bp)}<p>{pct(item.completed_bp)} of goal</p><meter min="0" max="10000" value={band(item.completed_bp)} aria-label="Target completion"></meter>{/if}
          {@render facts(item, ['status', 'days_elapsed', 'days_remaining', 'unknown_value_count', 'incomplete'])}
        </li>
      {/each}</ul>
    </section>
  {/each}
{/snippet}

{#snippet replayView(data: Row)}
  <div class="measure-grid">{#each [['Recorded then', 'then'], ['Current records', 'now']] as [title, key]}
    {@const snapshot = row(data[key])}
    <section class="currency"><h3>{title} · {text(snapshot.date)}</h3>
      {@render facts(snapshot, ['history_covers_date', 'earliest_history', 'unsupported_history_filters'])}
      {#if !rows(snapshot.currencies).length}<p class="empty">No figures in this snapshot.</p>{/if}
      {#each rows(snapshot.currencies) as group}
        {@const c = text(group.currency)}
        <section><h4>{c}</h4>{@render facts(group, ['pipeline_minor', 'weighted_minor', 'unknown_probability_count'], c)}
          <dl class="facts">{#each [['Won to date', 'won_to_date'], ['Lost to date', 'lost_to_date'], ['On hold', 'on_hold']] as [name, field]}<div><dt>{name}</dt><dd>{money(row(group[field]).total_minor, c)}</dd></div>{/each}</dl>
          {#if rows(group.stages).length}<details><summary>Stage breakdown</summary><ul class="plain ledger">{#each rows(group.stages) as stage}<li>{text(stage.stage) || text(stage.name)}{@render facts(stage, ['count', 'total_minor', 'unknown_value_count'], c)}</li>{/each}</ul></details>{/if}
        </section>
      {/each}
    </section>
  {/each}</div>
  {#each Object.entries(row(data.changes)) as [code, values]}<section class="currency"><h3>{code} · Change since selected day</h3><dl class="facts">{#each Object.entries(row(values)) as [key, value]}<div><dt>{label(key)}</dt><dd>{delta(value)}</dd></div>{/each}</dl></section>{/each}
  {@render notes(data.notes)}
{/snippet}

{#snippet reportView(data: Row)}
  {@render facts(row(data.definition), columns.reports)}
  {@render facts(data, ['as_of', 'records'])}
  {@render facts(row(data.period), ['label', 'from', 'to'])}
  {#if !rows(data.groups).length}<p class="empty">This report returned no result groups.</p>{/if}
  {#each rows(data.groups) as group}
    {@const c = text(group.currency)}
    <section class="currency"><h3>{c || 'Report results'}</h3>
      <div class="table-wrap"><table><thead><tr><th scope="col">Group</th>{#each list(data.columns) as col}<th scope="col">{text(col)}</th>{/each}</tr></thead>
      <tbody>{#each rows(group.rows) as result}<tr><th scope="row">{text(result.label)}{@render ids(text(result.record_kind), result.record_ids && community(result.record_kind, list(result.record_ids)[0]) ? result.record_ids : [])}</th>
        {#each rows(result.cells) as cell}<td>{text(cell.text) || (cell.money ? money(cell.value, c) : scalar(cell.value, '', ''))}</td>{/each}
      </tr>{/each}</tbody>
      {#if rows(row(group.total).cells).length}<tfoot><tr><th scope="row">{text(row(group.total).label) || 'Total'}</th>{#each rows(row(group.total).cells) as cell}<td>{text(cell.text) || (cell.money ? money(cell.value, c) : scalar(cell.value, '', ''))}</td>{/each}</tr></tfoot>{/if}
      </table></div>
    </section>
  {/each}
  {@render notes(data.notes)}
{/snippet}

{#snippet trace(data: Row)}
  {@const x = row(data.execution)}
  {@const record = row(x.record)}
  <p class="eyebrow">Recorded outcome</p><h3 class="outcome">{label(text(data.outcome)) || 'Outcome unavailable'}</h3>
  <p>{text(x.rule_name)}{#if text(record.kind)} · {label(text(record.kind))} {@render reference(record.kind, record.id)}{/if}</p>
  {@render facts(data, ['elapsed_ms'])}{@render facts(x, columns.runs)}
  {#if Object.keys(row(data.action_counts)).length}<h4>Actions by result</h4>{@render facts(row(data.action_counts), Object.keys(row(data.action_counts)))}{/if}
  <h4>Why it ran</h4>
  {#if present(x.event)}{@render evidence(row(x.event), ['type', 'kind', 'entity_kind', 'entity_id', 'event', 'at', 'occurred_at', 'source', 'actor', 'action'])}{/if}
  {#if present(x.playbook)}<h5>Playbook inputs</h5>{@render evidence(row(x.playbook), Object.keys(row(x.playbook)))}{/if}
  {#if !rows(x.evidence).length}<p class="muted">No condition evidence was recorded for this execution.</p>{/if}
  <ol class="plain trace">{#each rows(x.evidence) as reason}<li><strong>{text(reason.field) || text(reason.path) || 'Condition'}</strong>{@render facts(reason, ['path', 'operator', 'status'])}
    {#if present(reason.actual)}<h5>Observed value</h5>{@render evidence(row(reason.actual), ['text', 'number', 'currency', 'date', 'bool', 'strings'])}{/if}
  </li>{/each}</ol>
  <h4>Actions and results</h4>
  {#if !rows(x.actions).length}<p class="muted">No action entries were recorded.</p>{/if}
  <ol class="plain trace">{#each rows(x.actions) as action}
    {@const definition = row(action.definition)}
    {@const result = row(action.result)}
    <li><strong>{label(text(definition.kind)) || 'Action'}</strong> · {label(text(action.status)) || 'Status unavailable'}
      {@render evidence(definition, ['text', 'field', 'value', 'delay_days', 'due_on', 'task_status', 'profile_id', 'profile_revision'])}
      {@render facts(action, ['position', 'depends_on', 'code', 'attempts', 'updated_at', 'expires_at', 'snoozed_until'])}
      {#if text(result.id)}<p>Result · {label(text(result.kind))} {@render reference(result.kind, result.id)}</p>{/if}
    </li>
  {/each}</ol>
{/snippet}

{#snippet detail(data: Row)}
  {#if op === 'opportunities'}
    {@const deal = row(data.opportunity)}
    {@render opportunity(deal, true)}
    <h4>Recorded timeline</h4>
    {#if !rows(data.history).length}<p class="empty">No stage history was returned.</p>{/if}
    <ol class="plain trace">
      {#each rows(data.history) as event}
        <li><div class="record-head"><strong>{label(text(event.type)) || 'Recorded change'}</strong>{#if text(event.at)}<time datetime={text(event.at)}>{text(event.at)}</time>{/if}</div>
          {@render facts(event, ['from_stage', 'to_stage', 'from_value_minor', 'to_value_minor', 'from_probability_bp', 'to_probability_bp', 'reason', 'source', 'recorded_at', 'revision'], text(event.currency) || text(deal.currency))}
        </li>
      {/each}
    </ol>
  {:else if op === 'sales-pipelines'}
    {@render facts(data, columns['sales-pipelines'])}<h4>Stages in order</h4>
    {#if !rows(data.stages).length}<p class="empty">No stages were returned.</p>{/if}
    <ol class="plain trace">{#each rows(data.stages) as stage}<li><strong>{text(stage.name)}</strong>{@render facts(stage, ['description', 'kind', 'position', 'default_probability_bp', 'expected_days', 'aging_days', 'required_fields', 'entry_requirement', 'exit_requirement'])}</li>{/each}</ol>
  {:else if op === 'reports'}{@render reportView(data)}
  {:else if op === 'runs'}{@render trace(data)}
  {:else if op === 'scenarios'}
    {@render facts(data, ['as_of', 'today', 'horizon'])}
    {@render notes(data.notes)}
    {#each rows(data.currencies) as group}
      {@const c = text(group.currency)}
      <section class="currency"><h3>{c} · Scenario figures</h3>{@render evidence(group, Object.keys(group).filter((key) => key !== 'currency'), c)}</section>
    {:else}<p class="empty">No scenario figures in this scope.</p>{/each}
  {:else}
    {@render facts(data, columns[op] ?? [])}
    {#if op === 'rules'}
      <h4>Trigger</h4>{@render facts(row(data.trigger), ['kind', 'entity', 'event'])}
      {#if present(data.condition)}<h4>Conditions</h4>{@render evidence(row(data.condition), ['op', 'field', 'value', 'upper', 'children'])}{/if}
      <h4>Configured actions</h4>{@render evidence(data, ['actions'])}
    {:else if op === 'playbooks'}
      <h4>Inputs and steps</h4>{@render evidence(data, ['dates', 'steps'])}
    {:else if op === 'schedules'}
      <h4>Timing</h4>{@render facts(row(data.timing), ['kind', 'timezone', 'start_date', 'local_time', 'interval_minutes', 'weekdays', 'day_of_month'])}
      {#if text(data.rule_id)}<p>Rule · {text(data.rule_id)}</p>{/if}
    {/if}
  {/if}
{/snippet}

<section class="professional" class:editorial={workspace === 'business'} aria-label={`${workspaceName[workspace]} workspace`}>
  <p class="eyebrow">Professional / {workspaceName[workspace]}</p>
  {#if !tab}
    <h2>{workspaceName[workspace]}</h2>
    <p class="empty">No operations in this workspace are available to this session.</p>
  {:else}
    <nav aria-label={`${workspaceName[workspace]} views`}>
      {#each available as item (item.id)}<button type="button" aria-current={item.id === op ? 'page' : undefined} onclick={() => select(item.id)}>{item.label}</button>{/each}
    </nav>
    <header class="masthead"><h2>{tab.label}</h2><p>{tab.intro}</p></header>
    {#if canSearch || canPeriod || canCurrency || op === 'replay'}
      <div class="filters" aria-label="View filters">
        {#if canSearch}<label>Search <input type="search" value={current.q} oninput={(e) => change({ q: e.currentTarget.value })} /></label>{/if}
        {#if canPeriod}<label>Period <select value={current.period} onchange={(e) => change({ period: e.currentTarget.value })}>
          <option value="">{op === 'pulse' ? 'This month (default)' : op === 'concentration' ? 'This year (default)' : 'Default period'}</option>
          {#each periods as period}<option value={period}>{period === 'last30' ? 'Last 30 days' : period === 'last90' ? 'Last 90 days' : label(period)}</option>{/each}
        </select></label>{/if}
        {#if canCurrency}<label>Currency code <input value={current.currency} placeholder="All currencies" maxlength="3" oninput={(e) => change({ currency: e.currentTarget.value })} /></label>{/if}
        {#if op === 'replay'}<label>Recorded as of <input type="date" value={current.date} onchange={(e) => change({ date: e.currentTarget.value })} /></label>{/if}
      </div>
    {/if}
    {#if current.id}<div class="detail-head"><button type="button" onclick={() => { filter = { ...current, id: '', title: '' }; }}>Back to {tab.label}</button><h3>{current.title}</h3></div>{/if}
    {#if !endpoint}
      <p class="empty">Choose the date you want to replay.</p>
    {:else if !result}
      <p role="status" class="empty">Loading {tab.label.toLowerCase()}…</p>
    {:else if result.error}
      <section class="load-error"><p role="alert">{result.error}</p><button type="button" onclick={() => retry++}>Try again</button></section>
    {:else}
      {@const data = row(result.data)}
      {#if current.id}
        {@render detail(data)}
      {:else if op === 'pulse'}{@render pulse(data)}
      {:else if op === 'forecast'}{@render forecast(data)}
      {:else if op === 'concentration'}{@render concentration(data)}
      {:else if op === 'briefing'}{@render briefing(data)}
      {:else if op === 'watchtower'}
        {@render facts(data, ['as_of', 'customers_examined'])}{@render genericList(rows(data.customers), op)}{@render notes(data.notes)}
      {:else if op === 'renewals'}{@render renewals(result.data)}
      {:else if op === 'targets'}{@render targets(result.data)}
      {:else if op === 'replay'}{@render replayView(data)}
      {:else}{@render genericList(rows(result.data), op)}{/if}
      {#if paged && (current.offset || result.more)}
        <nav class="pagination" aria-label="Professional result pages">
          <span>Showing {current.offset + 1}–{current.offset + list(result.data).length}</span>
          <button type="button" disabled={current.offset === 0} onclick={() => change({ offset: Math.max(0, current.offset - 50) })}>Previous</button>
          <button type="button" disabled={!result.more || !list(result.data).length} onclick={() => change({ offset: current.offset + list(result?.data).length })}>Next</button>
        </nav>
      {:else if !paged && !current.id && result.more}<p class="muted">More records may exist. This view displays the latest results available from the server.</p>{/if}
    {/if}
  {/if}
</section>

<style>
  .professional { --pro-line: var(--line, #a1afb0); --pro-accent: var(--accent, #276a65); --pro-muted: var(--muted, #53676e); max-width: 76rem; min-width: 0; overflow-wrap: anywhere; }
  nav, .filters, .record-head, .detail-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .6rem 1rem; }
  nav { border-bottom: 1px solid var(--pro-line); padding: .15rem 0 .55rem; }
  nav button { border-color: transparent; }
  nav button[aria-current] { border-bottom: 2px solid var(--pro-accent); font-weight: 750; }
  .eyebrow { text-transform: uppercase; letter-spacing: .1em; font-size: .78rem; font-weight: 750; }
  .masthead { padding: clamp(.7rem, 2vw, 1.6rem) 0 .7rem; }
  .masthead h2, .edition h3, .outcome { margin: 0; font-size: clamp(1.6rem, 4vw, 2.7rem); }
  .editorial .masthead h2, .edition h3 { font-family: Georgia, serif; font-weight: 500; }
  .masthead p, .edition p, .detail-head h3 { margin: .4rem 0; }
  .filters { border-block: 1px solid var(--pro-line); padding: .55rem 0; }
  .filters label { display: flex; flex-direction: column; max-width: 100%; min-width: 0; gap: .2rem; font-size: .82rem; font-weight: 600; }
  .filters input, .filters select { max-width: 100%; }
  .filters input[maxlength] { width: 9rem; }
  .detail-head { padding: 1rem 0 .5rem; }
  .record-head { justify-content: space-between; }
  .record-head h3, .ledger h4 { margin: 0; }
  .plain { list-style: none; padding: 0; margin: .35rem 0; }
  .ledger > li { padding: .8rem 0; border-block-end: 1px solid var(--pro-line); }
  .ledger > li p { margin: .35rem 0; }
  .trace { border-inline-start: 2px solid var(--pro-accent); padding-inline-start: .9rem; }
  .trace > li { padding: .55rem 0; }
  .trace > li + li { border-top: 1px solid var(--pro-line); }
  .facts, .metrics { display: flex; flex-wrap: wrap; gap: .35rem 1.5rem; margin: .55rem 0; }
  .facts > div { max-width: 100%; }
  .facts dt, .metrics dt { color: var(--pro-muted); font-size: .82rem; }
  .facts dd, .metrics dd { margin: 0; white-space: pre-wrap; }
  .evidence { display: grid; }
  .evidence .evidence { margin-inline-start: .5rem; }
  .links { display: flex; flex-wrap: wrap; gap: .35rem 1rem; }
  .figure, .metrics dd, td, meter { font-variant-numeric: tabular-nums; }
  .figure { font-weight: 750; }
  .edition { padding: .9rem 0 1.3rem; border-bottom: 2px solid var(--pro-accent); }
  .currency { padding: .5rem 0 1.1rem; border-bottom: 1px solid var(--pro-line); }
  .currency h3 { margin: .65rem 0; }
  .metrics, .measure-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 13rem), 1fr)); gap: .7rem 1.25rem; }
  .measure-grid > section { min-width: 0; }
  .metrics > div { padding: .55rem 0; border-top: 1px solid var(--pro-line); }
  .metrics dd:first-of-type { font-size: 1.35rem; font-family: Georgia, serif; }
  .muted, .empty, .notes { color: var(--pro-muted); }
  .notes { padding-inline-start: 1.2rem; }
  details { margin: .65rem 0; }
  summary { cursor: pointer; font-weight: 650; }
  .table-wrap { overflow: auto; margin: .5rem 0; }
  table { width: 100%; border-collapse: collapse; text-align: start; }
  th, td { border-bottom: 1px solid var(--pro-line); padding: .45rem .65rem; text-align: start; }
  td { white-space: nowrap; }
  meter { width: min(100%, 28rem); accent-color: var(--pro-accent); }
  .pagination { align-items: center; margin-top: 1rem; }
  .load-error { border-inline-start: 2px solid var(--pro-accent); padding-inline-start: .8rem; margin: 1rem 0; }
</style>
