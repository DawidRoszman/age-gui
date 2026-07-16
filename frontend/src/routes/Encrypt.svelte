<script lang="ts">
  /** Encrypt a file for contacts, or under a passphrase. */
  import { onMount } from 'svelte'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import {
    contacts as contactsApi,
    groups as groupsApi,
    keys as keysApi,
    crypto,
    message,
    hasCode,
    Code,
    newJobId,
    EVENT_PROGRESS,
    EVENT_FILES_DROPPED,
    type Contact,
    type Group,
    type ProgressEvent,
  } from '../lib/api'
  import GroupForm from '../lib/GroupForm.svelte'

  let { announce, onLocked }: { announce: (m: string) => void; onLocked: () => void } =
    $props()

  let all = $state<Contact[]>([])
  let allGroups = $state<Group[]>([])
  let selected = $state<Set<string>>(new Set())
  // Include the user's own key so they can open the file. On by default: the
  // usual regret is encrypting something and then being unable to read it.
  let includeSelf = $state(true)
  let selfKeyType = $state('')
  let input = $state('')
  let inputName = $state('')
  let mode = $state<'contacts' | 'passphrase'>('contacts')
  let passphrase = $state('')
  let confirm = $state('')
  let query = $state('')
  let busy = $state(false)
  let jobId = $state('')
  let percent = $state(0)
  let error = $state('')
  let done = $state('')
  let dropActive = $state(false)
  let showSaveGroup = $state(false)

  const filtered = $derived(
    query.trim() === ''
      ? all
      : all.filter((c) =>
          (c.name + ' ' + c.note).toLowerCase().includes(query.trim().toLowerCase())
        )
  )

  // The distinct key kinds in the current recipient set, self included. age
  // refuses to put a quantum-resistant key in the same file as a classic one,
  // so we detect the mix here and block it with an explanation rather than let
  // the encrypt fail with an opaque error.
  const kinds = $derived.by(() => {
    const s = new Set<string>()
    for (const c of all) if (selected.has(c.id)) s.add(c.keyType)
    if (includeSelf && selfKeyType) s.add(selfKeyType)
    return s
  })
  const mixedKinds = $derived(kinds.has('hybrid-pq') && kinds.has('x25519'))

  const recipientCount = $derived(selected.size + (includeSelf ? 1 : 0))

  const canRun = $derived(
    !!input &&
      !busy &&
      (mode === 'contacts'
        ? recipientCount > 0 && !mixedKinds
        : passphrase.length > 0 && passphrase === confirm)
  )

  async function loadContactsAndGroups() {
    try {
      ;[all, allGroups] = await Promise.all([contactsApi.list(), groupsApi.list()])
    } catch (e) {
      error = message(e)
    }
  }

  onMount(() => {
    loadContactsAndGroups()
    keysApi
      .status()
      .then((s) => (selfKeyType = s.keyType))
      .catch(() => {})

    // Drops arrive from Go as absolute paths — file bytes never enter the
    // webview, which is what lets a 2 GB file work here at all.
    EventsOn(EVENT_FILES_DROPPED, (paths: string[]) => {
      if (paths?.length) pick(paths[0])
    })
    EventsOn(EVENT_PROGRESS, (p: ProgressEvent) => {
      if (p.jobId === jobId) percent = p.percent
    })

    return () => {
      EventsOff(EVENT_FILES_DROPPED)
      EventsOff(EVENT_PROGRESS)
    }
  })

  async function pick(path: string) {
    input = path
    done = ''
    error = ''
    inputName = await crypto.baseName(path)
  }

  async function browse() {
    try {
      const paths = await crypto.pickFiles('Choose a file to encrypt')
      if (paths.length) await pick(paths[0])
    } catch (e) {
      error = message(e)
    }
  }

  function toggle(id: string) {
    // Reassign so Svelte sees the change; mutating a Set in place does not
    // trigger reactivity.
    const next = new Set(selected)
    next.has(id) ? next.delete(id) : next.add(id)
    selected = next
  }

  function clearSelection() {
    selected = new Set()
  }

  // A group's members that still exist as contacts. Resolving against the live
  // list means a contact deleted since the group was made simply drops out.
  function resolvedMembers(g: Group): string[] {
    const ids = new Set(all.map((c) => c.id))
    return g.memberIds.filter((id) => ids.has(id))
  }

  // How much of a group is currently selected: all, some, or none. Drives the
  // group checkbox's checked / indeterminate state.
  function groupState(g: Group): 'all' | 'some' | 'none' {
    const m = resolvedMembers(g)
    if (m.length === 0) return 'none'
    const inSel = m.filter((id) => selected.has(id)).length
    if (inSel === 0) return 'none'
    return inSel === m.length ? 'all' : 'some'
  }

  // Clicking a group adds all its members, or removes them if they are already
  // all selected. The file is always encrypted to whoever is selected at that
  // moment — the group is only a shortcut for building that set.
  function toggleGroup(g: Group) {
    const m = resolvedMembers(g)
    if (m.length === 0) return
    const next = new Set(selected)
    const haveAll = m.every((id) => next.has(id))
    for (const id of m) haveAll ? next.delete(id) : next.add(id)
    selected = next
  }

  async function saveAsGroup(name: string, memberIds: string[]) {
    await groupsApi.create(name, memberIds)
    await loadContactsAndGroups()
    announce(`${name} saved.`)
  }

  async function run(outOverride = '') {
    busy = true
    error = ''
    done = ''
    percent = 0
    jobId = newJobId()
    try {
      const out =
        mode === 'contacts'
          ? await crypto.encrypt(jobId, input, outOverride, [...selected], includeSelf)
          : await crypto.encryptWithPassphrase(jobId, input, outOverride, passphrase)
      done = out
      passphrase = ''
      confirm = ''
      announce('Encrypted.')
    } catch (e) {
      if (hasCode(e, Code.TargetExists)) {
        // Ask rather than overwrite. The save dialog then confirms any
        // replacement itself, so the retry is allowed to replace.
        const suggested = await crypto.suggestEncryptOutput(input)
        const chosen = await crypto.chooseSavePath('Save encrypted file as', suggested)
        if (chosen) {
          busy = false
          return run(chosen)
        }
        error = ''
      } else if (hasCode(e, Code.Locked)) {
        // includeSelf reads the user's own key, which an idle auto-lock could
        // drop mid-encrypt. Send them to unlock rather than show a dead end.
        onLocked()
      } else {
        error = message(e)
        announce(error)
      }
    } finally {
      busy = false
      percent = 0
    }
  }

  async function cancel() {
    if (jobId) await crypto.cancel(jobId)
  }

  // Failing to open a file manager does not undo a successful encryption, so
  // report it without disturbing the success message above it.
  async function showDone() {
    try {
      await crypto.showInFolder(done)
    } catch (e) {
      error = message(e)
      announce(error)
    }
  }
</script>

<header>
  <h2>Encrypt a file</h2>
  <p class="lede">Lock a file so only the people you choose can open it.</p>
</header>

<div class="stack">
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <section
    class="card drop"
    class:active={dropActive}
    style="--wails-drop-target: drop"
    ondragenter={() => (dropActive = true)}
    ondragleave={() => (dropActive = false)}
    ondrop={() => (dropActive = false)}
  >
    {#if input}
      <p class="filename">{inputName}</p>
      <p class="hint">{input}</p>
      <button onclick={browse}>Choose a different file…</button>
    {:else}
      <p><strong>Drop a file here</strong></p>
      <p class="hint">or</p>
      <button class="primary" onclick={browse}>Choose a file…</button>
    {/if}
  </section>

  <section class="card stack">
    <fieldset>
      <legend>Who should be able to open it?</legend>
      <label class="radio">
        <input type="radio" bind:group={mode} value="contacts" />
        <span>Specific people <span class="hint">(recommended)</span></span>
      </label>
      <label class="radio">
        <input type="radio" bind:group={mode} value="passphrase" />
        <span>Anyone with a passphrase</span>
      </label>
    </fieldset>

    {#if mode === 'contacts'}
      <!-- Include-me stands apart from the contact list: the user is not their
           own contact, and encrypting only to yourself is a valid choice. -->
      <label class="person self">
        <input type="checkbox" bind:checked={includeSelf} />
        <span><strong>Let me open this file</strong></span>
        <span class="hint">encrypts to your own key too</span>
      </label>

      {#if all.length === 0}
        <p class="alert warn">
          You have no contacts yet. You can still encrypt this just for yourself
          above, or add someone's public key on the Contacts screen.
        </p>
      {:else}
        {#if allGroups.length > 0}
          <fieldset class="groups">
            <legend>Groups</legend>
            {#each allGroups as g (g.id)}
              {@const st = groupState(g)}
              <label class="person" class:disabled={resolvedMembers(g).length === 0}>
                <input
                  type="checkbox"
                  checked={st === 'all'}
                  indeterminate={st === 'some'}
                  disabled={resolvedMembers(g).length === 0}
                  onchange={() => toggleGroup(g)}
                />
                <span>{g.name}</span>
                <span class="hint">
                  {resolvedMembers(g).length} {resolvedMembers(g).length === 1 ? 'person' : 'people'}
                </span>
              </label>
            {/each}
          </fieldset>
        {/if}

        <div class="field">
          <label for="rsearch" class="sr-only">Filter people</label>
          <input
            id="rsearch"
            type="text"
            placeholder="Filter people…"
            bind:value={query}
          />
        </div>

        <ul class="people">
          {#each filtered as c (c.id)}
            <li>
              <label class="person">
                <input
                  type="checkbox"
                  checked={selected.has(c.id)}
                  onchange={() => toggle(c.id)}
                />
                <span>{c.name}</span>
                <code class="abbrev">{c.abbrev}</code>
              </label>
            </li>
          {:else}
            <li class="hint">Nothing matches “{query}”.</li>
          {/each}
        </ul>
      {/if}

      {#if mixedKinds}
        <p class="alert warn">
          Your selection mixes quantum-resistant and classic keys, which can't go
          in one file. Everyone — including you — must use the same kind of key.
          Your own key is quantum-resistant, so untick the classic contacts, or
          untick “Let me open this file”.
        </p>
      {/if}

      <div class="summary">
        <span>
          {#if recipientCount === 0}
            No one selected yet.
          {:else}
            Encrypting for <strong>{recipientCount}</strong>
            {recipientCount === 1 ? 'recipient' : 'recipients'}{#if includeSelf}
              (including you){/if}.
          {/if}
        </span>
        <div class="spacer"></div>
        {#if selected.size > 0}
          <button type="button" class="ghost" onclick={clearSelection}>Clear</button>
          <button type="button" class="ghost" onclick={() => (showSaveGroup = true)}>
            Save these as a group…
          </button>
        {/if}
      </div>
    {:else}
      <p class="alert warn">
        You'll have to send this passphrase to the other person somehow — and
        that channel is exactly what encryption is protecting you from.
        Encrypting to someone's public key avoids the problem entirely.
      </p>
      <div class="field">
        <label for="pw">Passphrase</label>
        <input id="pw" type="password" bind:value={passphrase} autocomplete="new-password" />
      </div>
      <div class="field">
        <label for="pw2">Type it again</label>
        <input id="pw2" type="password" bind:value={confirm} autocomplete="new-password" />
        {#if confirm && passphrase !== confirm}
          <p class="hint" role="alert" style="color: var(--danger)">These don't match yet.</p>
        {/if}
      </div>
    {/if}
  </section>

  {#if busy}
    <section class="card stack">
      <label for="prog">Encrypting…</label>
      <progress id="prog" max="100" value={percent}></progress>
      <button onclick={cancel}>Cancel</button>
    </section>
  {/if}

  {#if error}
    <p class="alert error" role="alert">{error}</p>
  {/if}
  {#if done}
    <div class="alert ok done">
      <p>Encrypted and saved to <code>{done}</code></p>
      <button class="ghost" onclick={showDone}>Show in folder</button>
    </div>
  {/if}

  <div class="row">
    <button class="primary" disabled={!canRun} onclick={() => run()}>Encrypt</button>
  </div>
</div>

<GroupForm
  bind:open={showSaveGroup}
  title="Save as a group"
  saveLabel="Create group"
  {all}
  initialMemberIds={[...selected]}
  onsave={saveAsGroup}
/>

<style>
  /* The path and the button share a row, wrapping on a narrow window rather
     than pushing the button off the edge. */
  .done {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex-wrap: wrap;
  }
  .done p { margin: 0; min-width: 0; overflow-wrap: anywhere; }

  header { margin-bottom: 1rem; }
  h2 { margin: 0 0 0.2rem; }
  .lede { margin: 0; color: var(--text-dim); }
  .drop {
    text-align: center;
    border-style: dashed;
    padding: 1.6rem;
  }
  .drop.active { border-color: var(--accent); background: var(--surface-2); }
  .drop p { margin: 0 0 0.5rem; }
  .filename { font-weight: 600; }
  fieldset { border: 0; padding: 0; margin: 0; }
  legend { font-weight: 600; padding: 0; margin-bottom: 0.4rem; }
  .radio, .person {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-weight: 400;
    margin: 0 0 0.3rem;
    cursor: pointer;
  }
  .people { list-style: none; margin: 0; padding: 0; }
  .abbrev { font-family: ui-monospace, monospace; font-size: 0.8rem; color: var(--text-dim); }
  .self {
    padding: 0.5rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface-2);
    margin-bottom: 0.6rem;
  }
  .groups {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.5rem 0.6rem;
    margin-bottom: 0.6rem;
  }
  .person.disabled { opacity: 0.5; cursor: default; }
  .summary {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-top: 0.6rem;
    padding-top: 0.6rem;
    border-top: 1px solid var(--border);
  }
  progress { width: 100%; }
  code {
    font-family: ui-monospace, monospace;
    background: var(--surface-2);
    padding: 0 0.25rem;
    border-radius: 4px;
    word-break: break-all;
  }
</style>
