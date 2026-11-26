<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import CommandMenu from './CommandMenu.svelte';
  import Dossier from './Dossier.svelte';
  import EditForm from './EditForm.svelte';
  import Pipeline from './Pipeline.svelte';
  import Professional from './Professional.svelte';
  import RecordAction from './RecordAction.svelte';
  import RecordDetail from './RecordDetail.svelte';
  import WorkspaceList from './WorkspaceList.svelte';
  import {
    APIError, descriptions, detail, errorMessage, hasSession, href, isContact, isSpace, labels, list, parseRoute,
    profile, proSpaces, readRoute, routeFor, sessionToken, singular, spaces, taskViews, titleOf, humanize, pageSize
  } from './api';
  import type {
    Activity, ContactDetail, Detail, Editable, EditRecord, Entry, Location, PipelineDetail, PipelineItem, Profile, Project, Ref, Route, Space, Stage, Task
  } from './api';

  type Theme = 'system' | 'light' | 'dark';
  type Density = 'comfortable' | 'compact';
  type Dialog =
    | { kind: 'commands' | 'capture' }
    | { kind: 'edit'; space: Editable; record: EditRecord | null; context: { entity?: Ref; project?: Ref } }
    | { kind: 'action'; action: { kind: 'complete'; task: Task } | { kind: 'move'; item: PipelineItem; stages: Stage[] } };

  let route = $state<Route>(readRoute());
  let me = $state<Profile | null>(null);
  let items = $state<Entry[]>([]);
  let next = $state<number | null>(null);
  let listState = $state<'loading' | 'ready' | 'failed'>('loading');
  let listError = $state('');
  let record = $state<Detail | null>(null);
  let recordState = $state<'idle' | 'loading' | 'ready' | 'failed'>('idle');
  let recordError = $state('');
  let dialog = $state<Dialog | null>(null);
  let message = $state('');
  let reload = $state(0);
  let searchField: HTMLInputElement | undefined = $state();
  let heading: HTMLElement | undefined = $state();
  let theme = $state<Theme>(read('tidecontacts-theme', ['system', 'light', 'dark'], 'system'));
  let density = $state<Density>(read('tidecontacts-density', ['comfortable', 'compact'], 'comfortable'));

  function read<T extends string>(key: string, allowed: T[], fallback: T): T {
    try {
      const value = localStorage.getItem(key);
      return allowed.includes(value as T) ? value as T : fallback;
    } catch { return fallback; }
  }
  function remember(key: string, value: string) {
    try { localStorage.setItem(key, value); } catch { /* Preferences stay session-only when storage is unavailable. */ }
  }

  const professional = $derived(me?.professional ?? []);
  const proWorkspaces = $derived([
    { space: 'sales' as const, label: 'Sales', needs: ['opportunities', 'sales-pipelines', 'customers', 'products'] },
    { space: 'business' as const, label: 'Business', needs: ['pulse', 'briefing', 'forecast', 'rescue', 'watchtower', 'concentration', 'renewals', 'targets', 'scenarios', 'replay', 'reports'] },
    { space: 'automation' as const, label: 'Automation', needs: ['rules', 'runs', 'approvals', 'playbooks', 'schedules'] }
  ].filter((entry) => entry.needs.some((need) => professional.includes(need))));
  const routes = $derived<{ space: Location; label: string }[]>([
    ...spaces.map((space) => ({ space, label: labels[space] })),
    ...proWorkspaces.map(({ space, label }) => ({ space, label })),
    { space: 'settings', label: 'Settings' }
  ]);
  const core = $derived(isSpace(route.space));
  const coreRoute = $derived(isSpace(route.space) ? route as Route<Space> : null);
  const editable = (space: Location): space is Editable => isSpace(space) && space !== 'pipelines';

  $effect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.dataset.density = density;
  });

  onMount(() => {
    const controller = new AbortController();
    if (hasSession) profile(controller.signal).then((value) => { me = value; }, () => { me = { demo: false, professional: [] }; });
    const pop = () => { route = readRoute(); };
    window.addEventListener('popstate', pop);
    return () => { controller.abort(); window.removeEventListener('popstate', pop); };
  });

  // A route that needs an unavailable Professional workspace falls back to People.
  $effect(() => {
    if (me && proSpaces.includes(route.space as never) && !proWorkspaces.some((entry) => entry.space === route.space)) go(routeFor('people'), true);
  });

  // The list reloads only for list-defining parts of the route, not when a record is selected.
  const listKey = $derived(isSpace(route.space) ? `${route.space}|${route.q}|${route.offset}|${route.view}|${route.sort}|${route.entity}|${route.project}` : '');
  const recordKey = $derived(isSpace(route.space) ? `${route.space}|${route.id}` : '');
  $effect(() => {
    void listKey;
    void reload;
    const current = untrack(() => coreRoute);
    if (!current || !hasSession) return;
    const controller = new AbortController();
    listState = 'loading';
    list(current, controller.signal).then((page) => {
      items = page.items ?? [];
      next = page.next_offset;
      listState = 'ready';
    }, (error) => {
      if (controller.signal.aborted) return;
      listError = errorMessage(error);
      listState = 'failed';
    });
    return () => controller.abort();
  });
  $effect(() => {
    void recordKey;
    void reload;
    const current = untrack(() => coreRoute);
    if (!current?.id || !hasSession) { record = null; recordState = 'idle'; return; }
    const controller = new AbortController();
    recordState = 'loading';
    detail(current, controller.signal).then(async (value) => {
      record = value;
      recordState = 'ready';
      await tick();
      heading?.focus({ preventScroll: true });
    }, (error) => {
      if (controller.signal.aborted) return;
      recordError = error instanceof APIError && error.code === 'not_found' ? 'This record is no longer available.' : errorMessage(error);
      recordState = 'failed';
    });
    return () => controller.abort();
  });

  function go(target: Route, replace = false) {
    const url = href(target);
    if (url !== location.pathname + location.search) history[replace ? 'replaceState' : 'pushState'](null, '', url);
    route = target;
    message = '';
  }
  function intercept(event: MouseEvent) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const anchor = (event.target as Element | null)?.closest('a');
    if (!anchor || anchor.target || anchor.hasAttribute('download') || anchor.origin !== location.origin) return;
    event.preventDefault();
    go(parseRoute(new URL(anchor.href)));
  }
  function typing(event: KeyboardEvent): boolean {
    const target = event.target as HTMLElement | null;
    return Boolean(target?.closest('input, textarea, select, dialog, [contenteditable]'));
  }
  function keys(event: KeyboardEvent) {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      dialog = { kind: 'commands' };
      return;
    }
    if (event.ctrlKey || event.metaKey || event.altKey || typing(event) || dialog) return;
    if (event.key === '/' && coreRoute) { event.preventDefault(); searchField?.focus(); }
    else if (event.key === 'c') { event.preventDefault(); dialog = { kind: 'capture' }; }
    else if (/^[1-6]$/.test(event.key)) go(routeFor(spaces[Number(event.key) - 1]));
  }

  function capture(space: Editable, context: { entity?: Ref; project?: Ref } = {}) {
    dialog = { kind: 'edit', space, record: null, context };
  }
  function related(space: 'tasks' | 'activities', ref: Ref) {
    capture(space, ref.kind === 'project' ? { project: ref } : { entity: ref });
  }
  function edit() {
    if (!coreRoute || !record || !editable(coreRoute.space)) return;
    const value = 'record' in record && !('items' in record) ? (record as ContactDetail).record : record as EditRecord;
    dialog = { kind: 'edit', space: coreRoute.space, record: value, context: {} };
  }
  function saved(space: Editable, value: EditRecord, created: boolean) {
    dialog = null;
    reload++;
    if (created) go({ ...routeFor(space), id: value.ID });
    message = created ? `Created ${singular[space]}.` : 'Changes saved.';
  }
  function acted(note: string) {
    dialog = null;
    message = note;
    reload++;
  }
  function search(event: Event) {
    if (!coreRoute) return;
    const q = (event.currentTarget as HTMLInputElement).value;
    go({ ...coreRoute, q, offset: 0, id: '', item: '' }, route.q !== '' || q === '');
  }

  const pipeline = $derived(record && 'items' in record ? record as PipelineDetail : null);
  const contact = $derived(record && 'record' in record && !('items' in record) ? record as ContactDetail : null);
  const plain = $derived(record && !pipeline && !contact ? record as Activity | Project | Task : null);
  const title = $derived(
    pipeline ? pipeline.record.Name : contact ? titleOf(contact.record) : plain ? titleOf(plain) : '');
  const task = $derived(plain && 'Due' in plain ? plain as Task : null);
  const showDetail = $derived(Boolean(coreRoute?.id));
</script>

<svelte:window onclick={intercept} onkeydown={keys} />
<a class="skip-link" href="#main">Skip to content</a>

<div class="app" class:has-detail={showDetail}>
  <header class="rail">
    <a class="brand" href="/people" aria-label="TideContacts"><span class="tide-mark" aria-hidden="true"></span><span>TideContacts</span></a>
    <nav aria-label="Workspaces">
      <p class="eyebrow">Relationships</p>
      <ul class="plain-list">
        {#each spaces as space, index (space)}
          <li><a href={href(routeFor(space))} aria-current={route.space === space ? 'page' : undefined}>
            <span>{labels[space]}</span><kbd>{index + 1}</kbd></a></li>
        {/each}
      </ul>
      {#if proWorkspaces.length}
        <p class="eyebrow">Professional</p>
        <ul class="plain-list">
          {#each proWorkspaces as { space, label } (space)}
            <li><a href={href(routeFor(space))} aria-current={route.space === space ? 'page' : undefined}><span>{label}</span></a></li>
          {/each}
        </ul>
      {/if}
    </nav>
    <div class="rail-foot">
      {#if me?.demo}<p class="sandbox"><span aria-hidden="true"></span>Fictional demo data</p>{/if}
      <a href="/settings" aria-current={route.space === 'settings' ? 'page' : undefined}>Settings</a>
    </div>
  </header>

  <main id="main" tabindex="-1">
    <div class="toolbar">
      <button type="button" class="command-open" onclick={() => { dialog = { kind: 'commands' }; }}>
        <span class="search-mark" aria-hidden="true"></span>Search &amp; commands <kbd>Ctrl K</kbd>
      </button>
      <button type="button" class="primary" onclick={() => { dialog = { kind: 'capture' }; }}>Capture <kbd>C</kbd></button>
    </div>
    {#if message}<p class="flash" role="status">{message}</p>{/if}

    {#if !hasSession}
      <section class="load-message">
        <h1>No session</h1>
        <p>Open TideContacts from its launcher. Each launch creates a private session for this window.</p>
      </section>
    {:else if route.space === 'settings'}
      <section class="settings scroll-pane">
        <h1 tabindex="-1">Settings</h1>
        <p class="muted">These choices are saved in this browser profile only.</p>
        <fieldset><legend>Appearance</legend>
          {#each ['system', 'light', 'dark'] as option (option)}
            <label class="choice"><input type="radio" name="theme" value={option} checked={theme === option}
              onchange={() => { theme = option as Theme; remember('tidecontacts-theme', theme); }} /> {humanize(option)}</label>
          {/each}
        </fieldset>
        <fieldset><legend>Density</legend>
          {#each ['comfortable', 'compact'] as option (option)}
            <label class="choice"><input type="radio" name="density" value={option} checked={density === option}
              onchange={() => { density = option as Density; remember('tidecontacts-density', density); }} /> {humanize(option)}</label>
          {/each}
        </fieldset>
        <section class="detail-section"><h3>Session</h3>
          <dl class="record-facts">
            <div><dt>Profile</dt><dd>{me ? (me.demo ? 'Fictional demo' : 'Local profile') : 'Checking…'}</dd></div>
            <div><dt>Professional</dt><dd>{professional.length ? `${professional.length} read operations authorized` : 'Not installed or not entitled'}</dd></div>
          </dl>
        </section>
        <section class="detail-section"><h3>Keyboard</h3>
          <dl class="record-facts">
            <div><dt>Search and commands</dt><dd><kbd>Ctrl</kbd> <kbd>K</kbd></dd></div>
            <div><dt>Quick capture</dt><dd><kbd>C</kbd></dd></div>
            <div><dt>Focus the list filter</dt><dd><kbd>/</kbd></dd></div>
            <div><dt>Switch workspace</dt><dd><kbd>1</kbd> – <kbd>6</kbd></dd></div>
          </dl>
        </section>
      </section>
    {:else if !core}
      <div class="scroll-pane">
        <Professional workspace={route.space as 'sales' | 'business' | 'automation'} capabilities={professional} token={sessionToken()} />
      </div>
    {:else if coreRoute}
      <div class="panes">
        <section class="list-pane" aria-label={`${labels[coreRoute.space]} list`}>
          <div class="pane-head">
            <h1>{labels[coreRoute.space]}</h1>
            <p class="muted">{descriptions[coreRoute.space]}</p>
            <div class="filters">
              <input type="search" bind:this={searchField} value={coreRoute.q} oninput={search} placeholder="Filter {labels[coreRoute.space].toLowerCase()}  ( / )" aria-label={`Filter ${labels[coreRoute.space]}`} />
              {#if coreRoute.space === 'tasks'}
                <select value={coreRoute.view} aria-label="Task view" onchange={(event) => go({ ...coreRoute, view: event.currentTarget.value, offset: 0 })}>
                  {#each taskViews as view (view)}<option value={view}>{humanize(view)}</option>{/each}
                </select>
              {/if}
              {#if isContact(coreRoute.space)}
                <select value={coreRoute.sort} aria-label="Sort" onchange={(event) => go({ ...coreRoute, sort: event.currentTarget.value, offset: 0 })}>
                  <option value="name">Name</option><option value="updated">Recently updated</option>
                </select>
              {/if}
              {#if editable(coreRoute.space)}
                <button type="button" class="quiet" onclick={() => capture(editable(coreRoute.space) ? coreRoute.space as Editable : 'tasks')}>New</button>
              {/if}
            </div>
            {#if coreRoute.entity || coreRoute.project}
              <p class="notice">Filtered to a related record. <a href={href({ ...coreRoute, entity: '', project: '', offset: 0 })}>Clear filter</a></p>
            {/if}
          </div>
          {#if listState === 'loading' && !items.length}<p class="muted pad" role="status">Loading…</p>
          {:else if listState === 'failed'}
            <div class="pad"><p class="error" role="alert">{listError}</p><button type="button" class="quiet" onclick={() => { reload++; }}>Try again</button></div>
          {:else if !items.length}
            <p class="muted pad">{coreRoute.q ? 'Nothing matches this filter.' : 'No records here yet.'}</p>
          {:else}
            <WorkspaceList route={coreRoute} {items} />
            {#if coreRoute.offset > 0 || next !== null}
              <nav class="pager" aria-label="Pages">
                <a class="quiet" aria-disabled={coreRoute.offset === 0} href={href({ ...coreRoute, offset: Math.max(0, coreRoute.offset - pageSize) })}>Previous</a>
                <span>{coreRoute.offset + 1}–{coreRoute.offset + items.length}</span>
                <a class="quiet" aria-disabled={next === null} href={next === null ? href(coreRoute) : href({ ...coreRoute, offset: next })}>Next</a>
              </nav>
            {/if}
          {/if}
        </section>

        <section class="detail-pane" aria-label="Record detail" aria-busy={recordState === 'loading'}>
          {#if !coreRoute.id}
            <div class="empty-detail"><p class="muted">Select a record to see its detail.</p></div>
          {:else if recordState === 'failed'}
            <div class="pad"><a class="back" href={href({ ...coreRoute, id: '', item: '' })}>‹ Back to {labels[coreRoute.space]}</a>
              <p class="error" role="alert">{recordError}</p></div>
          {:else if record}
            <article>
              <a class="back" href={href({ ...coreRoute, id: '', item: '' })}>‹ {labels[coreRoute.space]}</a>
              <div class="record-head">
                <h2 tabindex="-1" bind:this={heading}>{title}</h2>
                <div class="actions">
                  {#if task && task.Status !== 'completed' && task.Status !== 'canceled' && !task.Archived}
                    <button type="button" class="quiet" onclick={() => { dialog = { kind: 'action', action: { kind: 'complete', task } }; }}>Complete</button>
                  {/if}
                  {#if editable(coreRoute.space)}<button type="button" class="quiet" onclick={edit}>Edit</button>{/if}
                </div>
              </div>
              {#if contact}
                <Dossier data={contact} space={coreRoute.space as 'people' | 'organizations'} capture={related} />
              {:else if pipeline}
                <Pipeline {pipeline} route={coreRoute} changeStage={(item, stages) => { dialog = { kind: 'action', action: { kind: 'move', item, stages } }; }} />
              {:else if plain}
                <RecordDetail record={plain} capture={related} />
              {/if}
            </article>
          {:else}
            <p class="muted pad" role="status">Loading record…</p>
          {/if}
        </section>
      </div>
    {/if}
  </main>
</div>

{#if dialog?.kind === 'commands' || dialog?.kind === 'capture'}
  <CommandMenu mode={dialog.kind} {routes} active={route.space} close={() => { dialog = null; }}
    navigate={(target) => { dialog = null; go(target); }}
    capture={(space) => capture(space)} />
{:else if dialog?.kind === 'edit'}
  <EditForm space={dialog.space} record={dialog.record} context={dialog.context} close={() => { dialog = null; }} {saved} />
{:else if dialog?.kind === 'action'}
  <RecordAction action={dialog.action} close={() => { dialog = null; }} saved={acted} />
{/if}
