<script lang="ts">
  import RefLinks from './RefLinks.svelte';
  import { href, titleOf } from './api';
  import type { PipelineDetail, PipelineItem, Route, Stage } from './api';

  let { pipeline, route, changeStage }: { pipeline: PipelineDetail; route: Route; changeStage: (item: PipelineItem, stages: Stage[]) => void } = $props();
  let stages = $derived(pipeline.record.Stages ?? []);
  let items = $derived(pipeline.items ?? []);
  let selected = $derived(items.find((item) => item.ID === route.item));
  let columns = $derived(stages.map((stage) => ({ ...stage, items: items.filter((item) => item.StageID === stage.ID) })));
  let unstaged = $derived(items.filter((item) => !stages.some((stage) => stage.ID === item.StageID)));
</script>

{#if pipeline.record.Description}<p class="record-text detail-subtitle">{pipeline.record.Description}</p>{/if}
{#if pipeline.record.Archived}<p class="notice">This pipeline is archived.</p>{/if}
{#if stages.length || unstaged.length}
  <div class="stage-board" aria-label="Pipeline stages">
    {#each columns as stage (stage.ID)}
      {@render column(stage.Name, stage.items)}
    {/each}
    {#if unstaged.length}{@render column('Stage unavailable', unstaged)}{/if}
  </div>
{:else}<p class="muted detail-section">No stages are configured for this pipeline.</p>{/if}

{#snippet column(name: string, entries: PipelineItem[])}
  <section class="stage-column" aria-label={`${name} stage`}>
    <h3>{name} <span aria-label={`${entries.length} items`}>{entries.length}</span></h3>
    {#if entries.length}
      <ul class="plain-list">
        {#each entries as item (item.ID)}
          <li><a class="stage-item" aria-current={route.item === item.ID ? 'true' : undefined} href={href({ ...route, item: item.ID })}>
            <strong>{titleOf(item)}</strong>
            {#if item.NextAction}<small>{item.NextAction}</small>{/if}
          </a></li>
        {/each}
      </ul>
    {:else}<p>No items in this stage.</p>{/if}
  </section>
{/snippet}

{#if selected}
  <section class="item-inspector detail-section" aria-label="Selected pipeline item">
    <div class="section-heading"><h3 tabindex="-1" data-item-heading>{titleOf(selected)}</h3><a href={href({ ...route, item: '' })}>Close item</a></div>
    <dl class="record-facts"><div><dt>Stage</dt><dd>{stages.find((stage) => stage.ID === selected?.StageID)?.Name || selected.Stage || 'Stage unavailable'}</dd></div>
      {#if selected.StageEnteredAt}<div><dt>Entered stage</dt><dd><time datetime={selected.StageEnteredAt}>{selected.StageEnteredAt.slice(0, 10)}</time></dd></div>{/if}
      {#if selected.NextAction}<div><dt>Next action</dt><dd>{selected.NextAction}</dd></div>{/if}
    </dl>
    {#if selected.Notes}<p class="record-text">{selected.Notes}</p>{/if}
    {#if !pipeline.record.Archived && stages.some((stage) => stage.ID !== selected?.StageID)}
      <button type="button" class="primary capture-inline" onclick={() => { if (selected) changeStage(selected, stages); }}>Move stage</button>
    {/if}
    <RefLinks title="People and organizations" records={selected.Entities} />
    <RefLinks title="Projects" records={selected.Projects} />
    <RefLinks title="Tasks" records={selected.Tasks} />
    <RefLinks title="Activities" records={selected.Activities} />
  </section>
{:else if route.item}
  <p class="notice">The selected item is not available in this pipeline. <a href={href({ ...route, item: '' })}>Clear selection</a></p>
{/if}
