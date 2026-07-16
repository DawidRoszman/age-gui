<script lang="ts">
  /** The address book: other people's public keys, and groups of them. */
  import { onMount } from 'svelte'
  import { contacts, groups, message, type Contact, type Group } from '../lib/api'
  import Dialog from '../lib/Dialog.svelte'
  import GroupForm from '../lib/GroupForm.svelte'

  let { announce }: { announce: (m: string) => void } = $props()

  let tab = $state<'people' | 'groups'>('people')

  let all = $state<Contact[]>([])
  let query = $state('')
  let error = $state('')
  let toast = $state('')
  let searchBox: HTMLInputElement | undefined = $state()

  // Groups
  let allGroups = $state<Group[]>([])
  let showGroupForm = $state(false)
  let editingGroup = $state<Group | null>(null)
  let pendingGroupDelete = $state<Group | null>(null)

  // Contact names for a group, resolved against the live contact list so a
  // member deleted elsewhere simply drops out rather than showing a stale id.
  function memberNames(g: Group): string[] {
    const byId = new Map(all.map((c) => [c.id, c.name]))
    return g.memberIds.map((id) => byId.get(id)).filter((n): n is string => !!n)
  }

  // Add form
  let showAdd = $state(false)
  let name = $state('')
  let publicKey = $state('')
  let note = $state('')
  let addError = $state('')
  let addBusy = $state(false)

  // Delete confirmation
  let pendingDelete = $state<Contact | null>(null)

  const filtered = $derived(
    query.trim() === ''
      ? all
      : all.filter((c) =>
          (c.name + ' ' + c.note).toLowerCase().includes(query.trim().toLowerCase())
        )
  )

  async function load() {
    try {
      ;[all, allGroups] = await Promise.all([contacts.list(), groups.list()])
      error = ''
    } catch (e) {
      error = message(e)
    }
  }

  onMount(load)

  function newGroup() {
    editingGroup = null
    showGroupForm = true
  }

  function editGroup(g: Group) {
    editingGroup = g
    showGroupForm = true
  }

  async function saveGroup(name: string, memberIds: string[]) {
    // Throws on failure so GroupForm shows the message inline and stays open.
    if (editingGroup) {
      await groups.update(editingGroup.id, name, memberIds)
      toast = `${name} updated.`
    } else {
      await groups.create(name, memberIds)
      toast = `${name} created.`
    }
    await load()
    announce(toast)
  }

  async function confirmGroupDelete() {
    const g = pendingGroupDelete
    if (!g) return
    pendingGroupDelete = null
    try {
      await groups.remove(g.id)
      await load()
      toast = `${g.name} removed.`
      announce(toast)
    } catch (e) {
      error = message(e)
    }
  }

  function onKeydown(e: KeyboardEvent) {
    // "/" focuses search, the way it does in most list UIs — but never while
    // the user is already typing into a field.
    const el = document.activeElement
    const typing =
      el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement
    if (e.key === '/' && !typing) {
      e.preventDefault()
      searchBox?.focus()
    }
  }

  function resetAdd() {
    name = ''
    publicKey = ''
    note = ''
    addError = ''
  }

  async function submitAdd(e: SubmitEvent) {
    e.preventDefault()
    if (addBusy) return
    addBusy = true
    addError = ''
    try {
      const c = await contacts.add(name, publicKey, note)
      await load()
      showAdd = false
      resetAdd()
      toast = `${c.name} added.`
      announce(toast)
    } catch (err) {
      addError = message(err)
    } finally {
      addBusy = false
    }
  }

  async function importFile() {
    addError = ''
    try {
      const c = await contacts.importFromFile(name || 'Unnamed', note)
      if (!c) return // cancelled
      await load()
      showAdd = false
      resetAdd()
      toast = `${c.name} added from file.`
      announce(toast)
    } catch (err) {
      addError = message(err)
    }
  }

  async function copyKey(c: Contact) {
    try {
      await contacts.copyPublicKey(c.id)
      toast = `${c.name}'s public key copied.`
      announce(toast)
    } catch (e) {
      error = message(e)
    }
  }

  async function confirmDelete() {
    const c = pendingDelete
    if (!c) return
    pendingDelete = null
    try {
      await contacts.remove(c.id)
      await load()
      toast = `${c.name} removed.`
      announce(toast)
    } catch (e) {
      error = message(e)
    }
  }

</script>

<svelte:window onkeydown={onKeydown} />

<header>
  <div class="row">
    <div>
      <h2>Contacts</h2>
      <p class="lede">People you can encrypt files for, and groups of them.</p>
    </div>
    <div class="spacer"></div>
    {#if tab === 'people'}
      <button class="primary" onclick={() => { resetAdd(); showAdd = true }}>
        Add a contact
      </button>
    {:else}
      <button class="primary" disabled={all.length === 0} onclick={newGroup}>
        New group
      </button>
    {/if}
  </div>

  <div class="tabs" role="tablist" aria-label="Contacts and groups">
    <button
      role="tab"
      class="tab"
      class:active={tab === 'people'}
      aria-selected={tab === 'people'}
      onclick={() => (tab = 'people')}
    >
      People
    </button>
    <button
      role="tab"
      class="tab"
      class:active={tab === 'groups'}
      aria-selected={tab === 'groups'}
      onclick={() => (tab = 'groups')}
    >
      Groups {#if allGroups.length}<span class="pill">{allGroups.length}</span>{/if}
    </button>
  </div>
</header>

{#if error}
  <p class="alert error" role="alert">{error}</p>
{/if}
{#if toast}
  <p class="alert ok">{toast}</p>
{/if}

{#if tab === 'people'}
  <div class="field">
    <label for="search">Search</label>
    <input
      id="search"
      type="text"
      placeholder="Filter by name…  (press / to jump here)"
      bind:this={searchBox}
      bind:value={query}
    />
  </div>

  {#if all.length === 0}
    <div class="card empty">
      <p><strong>No contacts yet.</strong></p>
      <p class="lede">
        Ask someone for their public key — it starts with <code>age1</code> — then
        add it here. After that you can encrypt files for them.
      </p>
    </div>
  {:else if filtered.length === 0}
    <p class="lede">Nothing matches “{query}”.</p>
  {:else}
    <ul class="list">
      {#each filtered as c (c.id)}
        <li class="card">
          <div class="row">
            <div class="who">
              <div class="row">
                <strong>{c.name}</strong>
                {#if c.keyType === 'hybrid-pq'}
                  <span class="badge pq">Quantum-resistant</span>
                {:else}
                  <span class="badge">Classic</span>
                {/if}
              </div>
              <!-- Abbreviated: a post-quantum key is ~2000 characters. -->
              <code class="abbrev">{c.abbrev}</code>
              {#if c.note}<p class="hint">{c.note}</p>{/if}
            </div>
            <div class="spacer"></div>
            <button onclick={() => copyKey(c)} aria-label={`Copy ${c.name}'s public key`}>
              Copy key
            </button>
            <button
              class="danger"
              onclick={() => (pendingDelete = c)}
              aria-label={`Remove ${c.name}`}
            >
              Remove
            </button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
{:else if allGroups.length === 0}
  <div class="card empty">
    <p><strong>No groups yet.</strong></p>
    <p class="lede">
      {#if all.length === 0}
        Add a few contacts first, then gather them into groups so you can encrypt
        for everyone at once.
      {:else}
        Gather the people you often encrypt for into a group, so you can pick them
        all in one click. Start with <strong>New group</strong>.
      {/if}
    </p>
  </div>
{:else}
  <ul class="list">
    {#each allGroups as g (g.id)}
      <li class="card">
        <div class="row">
          <div class="who">
            <strong>{g.name}</strong>
            <p class="hint">
              {#if g.memberCount === 0}
                No members yet
              {:else}
                {memberNames(g).join(', ')}
              {/if}
            </p>
          </div>
          <div class="spacer"></div>
          <span class="badge">{g.memberCount} {g.memberCount === 1 ? 'person' : 'people'}</span>
          <button onclick={() => editGroup(g)} aria-label={`Edit ${g.name}`}>Edit</button>
          <button
            class="danger"
            onclick={() => (pendingGroupDelete = g)}
            aria-label={`Remove ${g.name}`}
          >
            Remove
          </button>
        </div>
      </li>
    {/each}
  </ul>
{/if}

<Dialog bind:open={showAdd} title="Add a contact">
  <form id="addform" class="stack" onsubmit={submitAdd}>
    <div class="field">
      <label for="cname">Name</label>
      <input id="cname" type="text" bind:value={name} required />
    </div>
    <div class="field">
      <label for="ckey">Their public key</label>
      <textarea
        id="ckey"
        rows="4"
        bind:value={publicKey}
        placeholder="age1…"
        aria-describedby="ckey-hint"
      ></textarea>
      <p id="ckey-hint" class="hint">
        Paste the key they sent you, or import the file instead.
      </p>
    </div>
    <div class="field">
      <label for="cnote">Note <span class="optional">(optional)</span></label>
      <input id="cnote" type="text" bind:value={note} />
    </div>
    {#if addError}
      <p class="alert error" role="alert">{addError}</p>
    {/if}
  </form>
  {#snippet footer()}
    <button type="button" onclick={importFile}>Import from a file…</button>
    <div class="spacer"></div>
    <button type="button" onclick={() => (showAdd = false)}>Cancel</button>
    <button class="primary" type="submit" form="addform" disabled={addBusy}>
      {addBusy ? 'Adding…' : 'Add contact'}
    </button>
  {/snippet}
</Dialog>

<Dialog
  open={pendingDelete !== null}
  onclose={() => (pendingDelete = null)}
  title="Remove this contact?"
>
  <p>
    Remove <strong>{pendingDelete?.name}</strong>? Files you already encrypted for
    them are not affected — you just won't be able to pick them until you add the
    key again.
  </p>
  {#snippet footer()}
    <button type="button" onclick={() => (pendingDelete = null)}>Cancel</button>
    <button class="primary danger" type="button" onclick={confirmDelete}>Remove</button>
  {/snippet}
</Dialog>

<GroupForm
  bind:open={showGroupForm}
  title={editingGroup ? 'Edit group' : 'New group'}
  saveLabel={editingGroup ? 'Save changes' : 'Create group'}
  {all}
  initialName={editingGroup?.name ?? ''}
  initialMemberIds={editingGroup?.memberIds ?? []}
  onsave={saveGroup}
/>

<Dialog
  open={pendingGroupDelete !== null}
  onclose={() => (pendingGroupDelete = null)}
  title="Remove this group?"
>
  <p>
    Remove the group <strong>{pendingGroupDelete?.name}</strong>? The contacts in
    it are not affected — only the group itself is deleted.
  </p>
  {#snippet footer()}
    <button type="button" onclick={() => (pendingGroupDelete = null)}>Cancel</button>
    <button class="primary danger" type="button" onclick={confirmGroupDelete}>Remove</button>
  {/snippet}
</Dialog>

<style>
  .tabs {
    display: flex;
    gap: 0.3rem;
    margin-top: 0.8rem;
    border-bottom: 1px solid var(--border);
  }
  .tab {
    background: transparent;
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    padding: 0.4rem 0.7rem;
    color: var(--text-dim);
    cursor: pointer;
  }
  .tab.active {
    color: var(--text);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }
  .pill {
    font-size: 0.72rem;
    background: var(--surface-2);
    border-radius: 999px;
    padding: 0 0.4rem;
    color: var(--text-dim);
  }
  header { margin-bottom: 1rem; }
  h2 { margin: 0 0 0.2rem; }
  .lede { margin: 0; color: var(--text-dim); }
  .list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.6rem; }
  .who { min-width: 0; display: flex; flex-direction: column; gap: 0.2rem; }
  .abbrev {
    font-family: ui-monospace, monospace;
    font-size: 0.82rem;
    color: var(--text-dim);
  }
  .empty { text-align: center; padding: 2rem; }
  .empty p { margin: 0 0 0.4rem; }
  .optional { font-weight: 400; color: var(--text-dim); }
  code {
    font-family: ui-monospace, monospace;
    background: var(--surface-2);
    padding: 0 0.25rem;
    border-radius: 4px;
  }
</style>
