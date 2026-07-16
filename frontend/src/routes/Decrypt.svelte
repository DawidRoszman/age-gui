<script lang="ts">
  /**
   * Decrypt a file.
   *
   * The app inspects the file first and asks for whatever it actually needs.
   * That check works while locked, because a passphrase-protected file needs no
   * key at all — demanding an unlock for it would be nonsense.
   */
  import { onMount } from 'svelte'
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
  import {
    crypto,
    message,
    hasCode,
    Code,
    newJobId,
    EVENT_PROGRESS,
    EVENT_FILES_DROPPED,
    type FileKind,
    type ProgressEvent,
  } from '../lib/api'

  let { announce, onLocked }: { announce: (m: string) => void; onLocked: () => void } =
    $props()

  let input = $state('')
  let inputName = $state('')
  let kind = $state<FileKind | ''>('')
  let passphrase = $state('')
  let busy = $state(false)
  let jobId = $state('')
  let percent = $state(0)
  let error = $state('')
  let done = $state('')
  let dropActive = $state(false)

  const canRun = $derived(
    !!input && !busy && (kind === 'recipients' || passphrase.length > 0)
  )

  onMount(() => {
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
    kind = ''
    passphrase = ''
    try {
      inputName = await crypto.baseName(path)
      // Ask the file what it needs before prompting for anything.
      kind = await crypto.inspect(path)
    } catch (e) {
      error = message(e)
    }
  }

  async function browse() {
    try {
      const paths = await crypto.pickFiles('Choose a file to decrypt')
      if (paths.length) await pick(paths[0])
    } catch (e) {
      error = message(e)
    }
  }

  async function run(outOverride = '') {
    busy = true
    error = ''
    done = ''
    percent = 0
    jobId = newJobId()
    try {
      const out =
        kind === 'passphrase'
          ? await crypto.decryptWithPassphrase(jobId, input, outOverride, passphrase)
          : await crypto.decrypt(jobId, input, outOverride)
      done = out
      passphrase = ''
      announce('Decrypted.')
    } catch (e) {
      if (hasCode(e, Code.TargetExists)) {
        const suggested = await crypto.suggestDecryptOutput(input)
        const chosen = await crypto.chooseSavePath('Save decrypted file as', suggested)
        if (chosen) {
          busy = false
          return run(chosen)
        }
        error = ''
      } else if (hasCode(e, Code.Locked)) {
        // Send the user back to the unlock screen rather than showing a dead end.
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

  // Failing to open a file manager does not undo a successful decryption, so
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
  <h2>Decrypt a file</h2>
  <p class="lede">Open a file that was encrypted for you.</p>
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
      <p><strong>Drop an encrypted file here</strong></p>
      <p class="hint">or</p>
      <button class="primary" onclick={browse}>Choose a file…</button>
    {/if}
  </section>

  {#if kind === 'recipients'}
    <p class="alert ok">This file was encrypted to a key. Your key will be used to open it.</p>
  {:else if kind === 'passphrase'}
    <section class="card stack">
      <p class="alert warn">This file is protected by a passphrase rather than a key.</p>
      <div class="field">
        <label for="pw">Passphrase</label>
        <input
          id="pw"
          type="password"
          bind:value={passphrase}
          autocomplete="current-password"
        />
        <p class="hint">Whoever encrypted the file chose this passphrase.</p>
      </div>
    </section>
  {/if}

  {#if busy}
    <section class="card stack">
      <label for="prog">Decrypting…</label>
      <progress id="prog" max="100" value={percent}></progress>
      <button onclick={cancel}>Cancel</button>
    </section>
  {/if}

  {#if error}
    <p class="alert error" role="alert">{error}</p>
  {/if}
  {#if done}
    <div class="alert ok done">
      <p>Decrypted and saved to <code>{done}</code></p>
      <button class="ghost" onclick={showDone}>Show in folder</button>
    </div>
  {/if}

  <div class="row">
    <button class="primary" disabled={!canRun} onclick={() => run()}>Decrypt</button>
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
  .drop { text-align: center; border-style: dashed; padding: 1.6rem; }
  .drop.active { border-color: var(--accent); background: var(--surface-2); }
  .drop p { margin: 0 0 0.5rem; }
  .filename { font-weight: 600; }
  progress { width: 100%; }
  code {
    font-family: ui-monospace, monospace;
    background: var(--surface-2);
    padding: 0 0.25rem;
    border-radius: 4px;
    word-break: break-all;
  }
</style>
