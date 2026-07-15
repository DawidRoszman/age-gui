<script lang="ts">
  /**
   * A modal built on the native <dialog> element.
   *
   * Using the platform element rather than a div means the browser handles the
   * focus trap, the backdrop, inertness of the page behind, and Escape — all
   * things hand-rolled modals get subtly wrong for keyboard and screen reader
   * users. We only add focus restoration, which <dialog> does not guarantee.
   */
  import type { Snippet } from 'svelte'

  let {
    open = $bindable(false),
    title,
    children,
    footer,
    onclose,
  }: {
    open?: boolean
    title: string
    children: Snippet
    footer?: Snippet
    onclose?: () => void
  } = $props()

  let el: HTMLDialogElement
  // Remembering the invoking element matters: without it, closing a dialog
  // dumps keyboard focus back to <body> and the user has to tab from the top.
  let restoreTo: HTMLElement | null = null

  $effect(() => {
    if (!el) return
    if (open && !el.open) {
      restoreTo = document.activeElement as HTMLElement | null
      el.showModal()
    } else if (!open && el.open) {
      el.close()
    }
  })

  function handleClose() {
    open = false
    onclose?.()
    restoreTo?.focus()
    restoreTo = null
  }
</script>

<dialog bind:this={el} onclose={handleClose} aria-label={title}>
  <div class="body">
    <h2>{title}</h2>
    {@render children()}
    <div class="footer">
      {#if footer}
        {@render footer()}
      {:else}
        <button type="button" onclick={() => (open = false)}>Close</button>
      {/if}
    </div>
  </div>
</dialog>

<style>
  dialog {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text);
    padding: 0;
    max-width: min(560px, 92vw);
    width: 100%;
  }
  dialog::backdrop {
    background: rgb(0 0 0 / 0.45);
  }
  .body { padding: 1.1rem; }
  h2 { margin: 0 0 0.8rem; font-size: 1.1rem; }
  .footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
    margin-top: 1.1rem;
  }
</style>
