<script lang="ts">
  import { refHref } from './api';
  import type { Dossier, Ref } from './api';
  let { dossier, name }: { dossier: Dossier; name: string } = $props();
  type Connection = { target: Ref; evidence: string; note?: string };
  const mapSize = 6;
  let display = $state<'map' | 'list'>('map');
  let offset = $state(0);
  let connections = $derived<Connection[]>([
    ...(dossier.Outgoing ?? []).map((edge) => ({ target: edge.Person, evidence: `Outgoing · ${edge.Type || 'Relationship'}`, note: edge.Notes })),
    ...(dossier.Incoming ?? []).map((edge) => ({ target: edge.Person, evidence: `Incoming · ${edge.Type || 'Relationship'}`, note: edge.Notes })),
    ...(dossier.Organizations ?? []).map((link) => ({ target: link.Organization, evidence: link.Current ? 'Organization · Current' : 'Organization · Former', note: link.Title })),
    ...(dossier.Projects ?? []).map((link) => ({ target: link.Project, evidence: 'Project', note: link.Status })),
    ...(dossier.Shared ?? []).map((edge) => ({ target: edge.Person, evidence: `Shared activities · ${edge.Count}`, note: edge.Notes }))
  ]);
  let page = $derived(connections.slice(offset, offset + mapSize));
  let height = $derived(Math.max(106, page.length * 53 + 20));
  const short = (s: string, length: number) => s.length > length ? s.slice(0, length - 1) + '…' : s;
</script>

<section class="detail-section" aria-label="Recorded relationships">
  <div class="section-heading">
    <h3>Recorded relationships</h3>
    {#if connections.length}
      <div class="segmented" aria-label="Relationship display">
        <button type="button" aria-pressed={display === 'map'} onclick={() => { display = 'map'; }}>Map</button>
        <button type="button" aria-pressed={display === 'list'} onclick={() => { display = 'list'; }}>List</button>
      </div>
    {/if}
  </div>
  {#if !connections.length}<p class="muted">No relationships are recorded in this dossier.</p>
  {:else if display === 'list'}
    <ul class="evidence-list">
      {#each connections as { target, evidence, note }}
        {@const url = refHref(target)}
        <li>
          {#if url}<a href={url}>{target.name || target.id}</a>{:else}<strong>{target.name || 'Unnamed record'}</strong>{/if}
          <small>{evidence}{note ? ` · ${note}` : ''}</small>
        </li>
      {/each}
    </ul>
  {:else}
    <figure class="relationship-map">
      <svg viewBox={`0 0 560 ${height}`} aria-label={`Recorded connections from ${name}`}>
        <title>Relationship map for {name}</title>
        {#each page as { target, evidence }, index}
          {@const y = 12 + index * 53}
          {@const url = refHref(target)}
          <path class="map-line" d={`M 160 ${height / 2} H 200 V ${y + 22} H 241`} />
          {#snippet node()}
            <rect x="241" y={y} width="306" height="44" rx="3" class="map-node" />
            <text x="253" y={y + 18} class="map-name">{short(target.name || target.id || 'Unnamed record', 37)}</text>
            <text x="253" y={y + 35} class="map-label">{short(evidence, 42)}</text>
          {/snippet}
          {#if url}<a href={url} aria-label={`${target.name || target.id} — ${evidence}`}>{@render node()}</a>{:else}{@render node()}{/if}
        {/each}
        <rect x="13" y={height / 2 - 24} width="147" height="48" rx="3" class="map-root" />
        <text x="24" y={height / 2 - 2} class="map-name">{short(name, 16)}</text>
        <text x="24" y={height / 2 + 15} class="map-label">Selected record</text>
      </svg>
      <figcaption>Lines connect this record to recorded evidence only.</figcaption>
    </figure>
    {#if connections.length > mapSize}
      <div class="pager">
        <button type="button" class="quiet" disabled={offset === 0} onclick={() => { offset -= mapSize; }}>Previous links</button>
        <span>{offset + 1}–{offset + page.length} of {connections.length}</span>
        <button type="button" class="quiet" disabled={offset + mapSize >= connections.length} onclick={() => { offset += mapSize; }}>Next links</button>
      </div>
    {/if}
  {/if}
</section>
