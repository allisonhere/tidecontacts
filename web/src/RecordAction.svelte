<script lang="ts">
  import Modal from './Modal.svelte';
  import { complete, errorMessage, move, titleOf } from './api';
  import type { PipelineItem, Stage, Task } from './api';

  let { action, close, saved }: {
    action: { kind: 'complete'; task: Task } | { kind: 'move'; item: PipelineItem; stages: Stage[] };
    close: () => void; saved: (message: string) => void;
  } = $props();
  let stage = $state('');
  let busy = $state(false);
  let failure = $state('');
  let movingTo = $derived(action.kind === 'move' ? action.stages.find((entry) => entry.ID === stage)?.Name : undefined);
  function dismiss() { if (!busy) close(); }
  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (busy || (action.kind === 'move' && !movingTo)) return;
    busy = true;
    failure = '';
    try {
      if (action.kind === 'complete') {
        await complete(action.task);
        saved(`${titleOf(action.task)} marked complete.`);
      } else {
        await move(action.item, stage);
        saved(`${titleOf(action.item)} moved to ${movingTo}.`);
      }
    } catch (error) { failure = errorMessage(error); }
    finally { busy = false; }
  }
</script>

<Modal title={action.kind === 'complete' ? 'Complete task' : 'Move pipeline item'} {dismiss}>
  <form class="editor" onsubmit={submit}>
    {#if action.kind === 'complete'}
      <p>Mark <strong>{titleOf(action.task)}</strong> complete?</p>
    {:else}
      <p>Move <strong>{titleOf(action.item)}</strong> from {action.stages.find((entry) => entry.ID === action.item.StageID)?.Name || action.item.Stage || 'its current stage'}.</p>
      <label>Destination stage <select bind:value={stage} data-initial required>
        <option value="">Choose a stage</option>
        {#each action.stages.filter((entry) => entry.ID !== action.item.StageID) as entry (entry.ID)}
          <option value={entry.ID}>{entry.Name}</option>
        {/each}
      </select></label>
    {/if}
    {#if failure}<p class="error" role="alert">{failure}</p>{/if}
    <div class="form-actions">
      <button type="button" class="quiet" disabled={busy} onclick={dismiss} data-initial={action.kind === 'complete' ? '' : undefined}>Cancel</button>
      <button type="submit" class="primary" disabled={busy || (action.kind === 'move' && !movingTo)}>{busy ? 'Saving…' : action.kind === 'complete' ? 'Complete task' : 'Move item'}</button>
    </div>
  </form>
</Modal>
