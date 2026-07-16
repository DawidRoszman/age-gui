<script lang="ts">
  /**
   * Create or edit a group: a name plus a searchable checklist of contacts.
   *
   * Shared by the Contacts "Groups" tab and the Encrypt screen's "save these as
   * a group" shortcut, so the two cannot drift apart. The caller owns
   * persistence via `onsave` — this component only gathers a name and a set of
   * member ids.
   */
  import { message, type Contact } from './api'
  import Dialog from './Dialog.svelte'

  let {
    open = $bindable(false),
    title,
    saveLabel = 'Save group',
    all,
    initialName = '',
    initialMemberIds = [],
    onsave,
  }: {
    open?: boolean
    title: string
    saveLabel?: string
    all: Contact[]
    initialName?: string
    initialMemberIds?: string[]
    onsave: (name: string, memberIds: string[]) => Promise<void>
  } = $props()

  let name = $state('')
  let selected = $state<Set<string>>(new Set())
  let query = $state('')
  let error = $state('')
  let busy = $state(false)

  // Re-seed each time the dialog opens, so editing one group never leaks its
  // values into the next. Keyed on `open` so it runs on the false→true edge.
  let wasOpen = false
  $effect(() => {
    if (open && !wasOpen) {
      name = initialName
      selected = new Set(initialMemberIds)
      query = ''
      error = ''
    }
    wasOpen = open
  })

  const filtered = $derived(
    query.trim() === ''
      ? all
      : all.filter((c) =>
          (c.name + ' ' + c.note).toLowerCase().includes(query.trim().toLowerCase())
        )
  )

  function toggle(id: string) {
    const next = new Set(selected)
    next.has(id) ? next.delete(id) : next.add(id)
    selected = next
  }

  async function save() {
    if (busy) return
    busy = true
    error = ''
    try {
      await onsave(name.trim(), [...selected])
      open = false
    } catch (e) {
      error = message(e)
    } finally {
      busy = false
    }
  }
</script>

<Dialog bind:open {title}>
  <div class="stack">
    <div class="field">
      <label for="group-name">Name</label>
      <input id="group-name" type="text" bind:value={name} placeholder="e.g. Team Alpha" />
    </div>

    <div class="field">
      <label for="group-search">People in this group</label>
      {#if all.length === 0}
        <p class="hint">
          You have no contacts yet. Add someone on the Contacts screen first, then
          you can group them.
        </p>
      {:else}
        <input
          id="group-search"
          type="text"
          placeholder="Filter contacts…"
          bind:value={query}
        />
        <p class="count hint">
          {selected.size} selected of {all.length}
        </p>
        <ul class="people">
          {#each filtered as c (c.id)}
            <li>
              <label class="person">
                <input
                  type="checkbox"
                  checked={selected.has(c.id)}
                  onchange={() => toggle(c.id)}
                />
                <span class="who">{c.name}</span>
                <code class="abbrev">{c.abbrev}</code>
              </label>
            </li>
          {:else}
            <li class="hint">Nothing matches “{query}”.</li>
          {/each}
        </ul>
      {/if}
    </div>

    {#if error}
      <p class="alert error" role="alert">{error}</p>
    {/if}
  </div>

  {#snippet footer()}
    <button type="button" onclick={() => (open = false)}>Cancel</button>
    <button class="primary" type="button" disabled={busy} onclick={save}>
      {busy ? 'Saving…' : saveLabel}
    </button>
  {/snippet}
</Dialog>

<style>
  .count {
    margin: 0.3rem 0 0.2rem;
  }
  .people {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 40vh;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }
  .people li {
    padding: 0.1rem 0.4rem;
  }
  .person {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
    padding: 0.15rem 0;
  }
  .who {
    flex-shrink: 0;
  }
  .abbrev {
    font-family: ui-monospace, monospace;
    font-size: 0.78rem;
    color: var(--text-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
