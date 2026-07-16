<script lang="ts">
  /** Encrypt a file for contacts, or under a passphrase. */
  import { onMount } from 'svelte'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import {
    contacts as contactsApi,
    crypto,
    message,
    hasCode,
    Code,
    newJobId,
    EVENT_PROGRESS,
    EVENT_FILES_DROPPED,
    type Contact,
    type ProgressEvent,
  } from '../lib/api'

  let { announce }: { announce: (m: string) => void } = $props()

  let all = $state<Contact[]>([])
  let selected = $state<Set<string>>(new Set())
  let input = $state('')
  let inputName = $state('')
  let mode = $state<'contacts' | 'passphrase'>('contacts')
  let passphrase = $state('')
  let confirm = $state('')
  let busy = $state(false)
  let jobId = $state('')
  let percent = $state(0)
  let error = $state('')
  let done = $state('')
  let dropActive = $state(false)

  const canRun = $derived(
    !!input &&
      !busy &&
      (mode === 'contacts'
        ? selected.size > 0
        : passphrase.length > 0 && passphrase === confirm)
  )

  onMount(() => {
    contactsApi
      .list()
      .then((c) => (all = c))
      .catch((e) => (error = message(e)))

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

  async function run(outOverride = '') {
    busy = true
    error = ''
    done = ''
    percent = 0
    jobId = newJobId()
    try {
      const out =
        mode === 'contacts'
          ? await crypto.encrypt(jobId, input, outOverride, [...selected])
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
      {#if all.length === 0}
        <p class="alert warn">
          You have no contacts yet. Add someone's public key on the Contacts
          screen first.
        </p>
      {:else}
        <ul class="people">
          {#each all as c (c.id)}
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
          {/each}
        </ul>
      {/if}
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
  progress { width: 100%; }
  code {
    font-family: ui-monospace, monospace;
    background: var(--surface-2);
    padding: 0 0.25rem;
    border-radius: 4px;
    word-break: break-all;
  }
</style>
