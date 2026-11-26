<script lang="ts">
  import { onMount } from 'svelte';
  import type { Snippet } from 'svelte';

  let { title, dismiss, children }: { title: string; dismiss: () => void; children: Snippet } = $props();
  let dialog: HTMLDialogElement;

  onMount(() => {
    const trigger = document.activeElement;
    dialog.showModal();
    dialog.querySelector<HTMLElement>('[data-initial]')?.focus();
    return () => {
      dialog.close();
      if (trigger instanceof HTMLElement && trigger.isConnected) trigger.focus({ preventScroll: true });
    };
  });
</script>

<dialog bind:this={dialog} aria-labelledby="modal-title" oncancel={(event) => { event.preventDefault(); dismiss(); }}>
  <div class="modal-heading">
    <h2 id="modal-title">{title}</h2>
    <button type="button" class="quiet" aria-label="Close dialog" onclick={dismiss}>Close <kbd>Esc</kbd></button>
  </div>
  {@render children()}
</dialog>
