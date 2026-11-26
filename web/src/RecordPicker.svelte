<script lang="ts">
  import { errorMessage, list, routeFor, titleOf } from './api';
  import type { Entry, Ref, Space } from './api';

  type Kind = 'people' | 'organizations' | 'projects';
  let { label, kinds, value, change, removable = true }: {
    label: string; kinds: Kind[]; value: Ref | null; change: (ref: Ref | null) => void; removable?: boolean;
  } = $props();
  const kindNames: Record<Kind, string> = { people: 'People', organizations: 'Organizations', projects: 'Projects' };
  const refs: Partial<Record<Space, string>> = { people: 'person', organizations: 'organization', projects: 'project' };
  const pickerID = $props.id();
  let opened = $state(false);
  let kind = $state<Kind>('people');
  let q = $state('');
  let offset = $state(0);
  let page = $state<Entry[]>([]);
  let next = $state<number | null>(null);
  let loading = $state(false);
  let failure = $state('');

  $effect(() => {
    if (!opened) return;
    const route = { ...routeFor(kind), q: q.trim(), offset };
    const controller = new AbortController();
    loading = true;
    failure = '';
    const timer = window.setTimeout(async () => {
      try {
        const result = await list(route, controller.signal, 10);
        if (controller.signal.aborted) return;
        page = (result.items ?? []).filter((record) => !('Archived' in record && record.Archived));
        next = result.next_offset;
      } catch (error) {
        if (!controller.signal.aborted) failure = errorMessage(error);
      } finally {
        if (!controller.signal.aborted) loading = false;
      }
    }, 160);
    return () => { window.clearTimeout(timer); controller.abort(); };
  });

  function toggle() {
    if (opened) { opened = false; return; }
    kind = kinds.find((key) => refs[key] === value?.kind) ?? kinds[0];
    q = '';
    offset = 0;
    opened = true;
  }
</script>

<fieldset class="picker">
  <legend>{label}</legend>
  <div class="actions">
    <span>{value?.name || (value?.id ? 'Linked record' : 'None selected')}</span>
    <button type="button" class="quiet" aria-expanded={opened} aria-controls={pickerID} onclick={toggle}>{opened ? 'Hide choices' : value ? 'Change' : 'Choose'}<span class="sr-only"> {label}</span></button>
    {#if value && removable}<button type="button" class="quiet" onclick={() => { change(null); opened = false; }}>Remove<span class="sr-only"> {label}</span></button>{/if}
  </div>
  {#if opened}
    <div id={pickerID} class="picker-body">
      {#if kinds.length > 1}
        <label>Record type <select value={kind} onchange={(event) => { kind = event.currentTarget.value as Kind; offset = 0; }}>
          {#each kinds as option}<option value={option}>{kindNames[option]}</option>{/each}
        </select></label>
      {/if}
      <label>Find {kindNames[kind].toLowerCase()} <input type="search" value={q} oninput={(event) => { q = event.currentTarget.value; offset = 0; }} /></label>
      {#if loading}<p role="status">Looking up records…</p>
      {:else if failure}<p class="error" role="alert">{failure}</p>
      {:else if !page.length}<p>No matching active records on this page.</p>
      {:else}
        <ul class="picker-results">
          {#each page as record (record.ID)}
            <li><button type="button" class="row-button" onclick={() => {
              change({ kind: refs[kind], id: record.ID, name: titleOf(record) });
              opened = false;
            }}>{titleOf(record)}</button></li>
          {/each}
        </ul>
      {/if}
      {#if !loading && !failure && (offset > 0 || next !== null)}
        <div class="actions">
          <button type="button" class="quiet" disabled={offset === 0} onclick={() => { offset = Math.max(0, offset - 10); }}>Previous</button>
          <button type="button" class="quiet" disabled={next === null} onclick={() => { if (next !== null) offset = next; }}>Next</button>
        </div>
      {/if}
    </div>
  {/if}
</fieldset>
