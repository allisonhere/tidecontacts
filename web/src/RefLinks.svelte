<script lang="ts">
  import { refHref } from './api';
  import type { Ref } from './api';
  let { title, records }: { title: string; records: Ref[] | null | undefined } = $props();
</script>

{#if records?.length}
  <section class="detail-section">
    <h3>{title}</h3>
    <ul class="evidence-list">
      {#each records as ref}
        {@const url = refHref(ref)}
        <li>
          {#if url}<a href={url}>{ref.name || ref.id}</a>{:else}<span>{ref.name || ref.id || 'Unavailable record'}</span>{/if}
          {#if ref.role}<small>{ref.role}</small>{/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
