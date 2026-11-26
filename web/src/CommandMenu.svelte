<script lang="ts">
  import { tick } from 'svelte';
  import Modal from './Modal.svelte';
  import { href, labels, list, routeFor, singular, spaces, titleOf } from './api';
  import type { Editable, Location, Route, Space } from './api';

  let { mode, routes, active, close, navigate, capture }: {
    mode: 'commands' | 'capture'; routes: { space: Location; label: string }[]; active: Location;
    close: () => void; navigate: (route: Route) => void; capture: (space: Editable) => void;
  } = $props();
  type Choice = { key: string; text: string; category: string; action: () => void };
  const editable = spaces.filter((space): space is Editable => space !== 'pipelines');
  let q = $state('');
  let query = $derived(q.trim().toLowerCase());
  let records = $state<Choice[]>([]);
  let listbox: HTMLUListElement;
  let search: HTMLInputElement;
  let loading = $state(false);
  let searchFailure = $state(false);
  let searching = $derived(mode === 'commands' && query.length > 1);
  let commands = $derived<Choice[]>([
    ...(mode === 'commands' ? routes.map(({ space, label }) => ({ key: `go-${space}`, text: `Go to ${label}`, category: 'Workspace', action: () => navigate(routeFor(space)) })) : []),
    ...editable.map((space) => ({ key: `new-${space}`, text: space === 'activities' ? 'Log activity' : `New ${singular[space]}`, category: 'Capture', action: () => capture(space) }))
  ]);
  let results = $derived(commands.filter((choice) => !query || choice.text.toLowerCase().includes(query)).concat(searching && !loading ? records : []));

  $effect(() => {
    const phrase = query;
    if (!searching) return;
    const ordered = [...spaces].sort((a, b) => Number(b === active) - Number(a === active));
    const controller = new AbortController();
    loading = true;
    searchFailure = false;
    const timer = window.setTimeout(async () => {
      const sources = await Promise.allSettled(ordered.map(async (space: Space) => {
        const found = await list({ ...routeFor(space), q: phrase, view: 'all' }, controller.signal, 4);
        return (found.items ?? []).map((entry) => {
          const route = { ...routeFor(space), id: entry.ID };
          return { key: href(route), text: titleOf(entry), category: labels[space], action: () => navigate(route) };
        });
      }));
      if (controller.signal.aborted) return;
      records = sources.flatMap((result) => result.status === 'fulfilled' ? result.value : []);
      searchFailure = sources.some((result) => result.status === 'rejected');
      loading = false;
    }, 190);
    return () => { controller.abort(); window.clearTimeout(timer); };
  });

  async function moveFocus(event: KeyboardEvent, index: number) {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    event.preventDefault();
    await tick();
    const buttons = listbox?.querySelectorAll('button');
    if (!buttons?.length) return;
    if (index === -1) buttons[event.key === 'ArrowDown' ? 0 : buttons.length - 1]?.focus();
    else {
      const next = index + (event.key === 'ArrowDown' ? 1 : -1);
      if (next < 0 || next >= buttons.length) search?.focus();
      else buttons[next]?.focus();
    }
  }
</script>

<Modal title={mode === 'commands' ? 'Commands & record search' : 'Quick capture'} dismiss={close}>
  <form class="editor" onsubmit={(event) => { event.preventDefault(); results[0]?.action(); }}>
    <label>{mode === 'commands' ? 'Find a command or record' : 'Find a record type'}
      <input type="search" bind:this={search} bind:value={q} data-initial onkeydown={(event) => moveFocus(event, -1)} />
    </label>
    {#if loading && searching}<p class="muted" role="status">Searching records…</p>{/if}
    {#if searchFailure && searching}<p class="muted">Some record searches could not be loaded.</p>{/if}
    {#if !results.length && (!loading || !searching)}<p class="muted">No matching {mode === 'commands' ? 'commands or records' : 'record types'}.</p>{/if}
    <ul class="command-results" bind:this={listbox}>
      {#each results as result, index (result.key)}
        <li><button type="button" class="row-button" onclick={result.action} onkeydown={(event) => moveFocus(event, index)}>
          <strong>{result.text}</strong><small>{result.category}</small>
        </button></li>
      {/each}
    </ul>
    <p class="muted"><kbd>↑</kbd> <kbd>↓</kbd> Select <kbd>Enter</kbd> Open <kbd>Esc</kbd> Close</p>
  </form>
</Modal>
