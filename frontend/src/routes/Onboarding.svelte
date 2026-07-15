<script lang="ts">
  /**
   * First run: create the keypair.
   *
   * The passphrase is the only thing standing between an attacker with the key
   * file and the user's secrets, and it cannot be reset — so this screen exists
   * mainly to make both of those facts land before the key is created.
   */
  import { keys, message, type KeyStatus } from '../lib/api'

  let { onDone }: { onDone: (s: KeyStatus) => void } = $props()

  // "restore" exists because a backup nobody can restore is not a backup. It
  // also covers someone arriving with an existing age-keygen key.
  let mode = $state<'choose' | 'create' | 'restore'>('choose')

  let passphrase = $state('')
  let confirm = $state('')
  let busy = $state(false)
  let error = $state('')
  let acknowledged = $state(false)

  const mismatch = $derived(confirm.length > 0 && passphrase !== confirm)
  const tooShort = $derived(passphrase.length > 0 && passphrase.length < 8)
  const ready = $derived(
    passphrase.length >= 8 && passphrase === confirm && acknowledged && !busy
  )

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!ready) return
    busy = true
    error = ''
    try {
      const status = await keys.generate(passphrase)
      // Drop the plaintext from JS state the moment it is no longer needed.
      passphrase = ''
      confirm = ''
      onDone(status)
    } catch (err) {
      error = message(err)
    } finally {
      busy = false
    }
  }

  async function submitRestore(e: SubmitEvent) {
    e.preventDefault()
    if (busy || !passphrase) return
    busy = true
    error = ''
    try {
      const status = await keys.restore(passphrase)
      if (!status) return // file dialog cancelled
      passphrase = ''
      onDone(status)
    } catch (err) {
      error = message(err)
    } finally {
      busy = false
    }
  }

  function back() {
    mode = 'choose'
    passphrase = ''
    confirm = ''
    error = ''
    acknowledged = false
  }
</script>

<main class="centered">
{#if mode === 'choose'}
  <div class="card stack">
    <h1>Welcome to Age GUI</h1>
    <p class="lede">
      This app locks files so only the people you choose can open them. To start,
      you need a key.
    </p>
    <div class="stack">
      <button class="primary big" onclick={() => (mode = 'create')}>
        <strong>Create a new key</strong>
        <span class="sub">I'm new to this</span>
      </button>
      <button class="big" onclick={() => (mode = 'restore')}>
        <strong>I already have a key</strong>
        <span class="sub">Restore from a backup, or import a key from age</span>
      </button>
    </div>
  </div>

{:else if mode === 'restore'}
  <form class="card stack" onsubmit={submitRestore}>
    <h1>Restore your key</h1>
    <p class="lede">
      Choose the backup file you saved from this app, or a key file made by
      <code>age-keygen</code>. Then enter its passphrase — for an age-keygen file,
      which has no passphrase yet, pick one now and it will be protected with it.
    </p>

    <div class="field">
      <label for="rpass">Passphrase</label>
      <input
        id="rpass"
        type="password"
        bind:value={passphrase}
        autocomplete="current-password"
        required
      />
    </div>

    {#if error}
      <p class="alert error" role="alert">{error}</p>
    {/if}

    <div class="row">
      <button type="button" onclick={back}>Back</button>
      <div class="spacer"></div>
      <button class="primary" type="submit" disabled={busy || !passphrase}>
        {busy ? 'Restoring…' : 'Choose file and restore'}
      </button>
    </div>
  </form>

{:else}
  <form class="card stack" onsubmit={submit}>
    <h1>Create your key</h1>
    <p class="lede">
      You'll share the <strong>public</strong> half with people who want to send you
      files. The <strong>private</strong> half stays on this computer, protected by
      a passphrase.
    </p>

    <div class="field">
      <label for="pass">Choose a passphrase</label>
      <input
        id="pass"
        type="password"
        bind:value={passphrase}
        autocomplete="new-password"
        aria-describedby="pass-hint"
        required
      />
      <p id="pass-hint" class="hint">
        At least 8 characters. A few random words is both stronger and easier to
        remember than one mangled word.
      </p>
      {#if tooShort}
        <p class="hint" role="alert" style="color: var(--danger)">
          That's shorter than 8 characters.
        </p>
      {/if}
    </div>

    <div class="field">
      <label for="confirm">Type it again</label>
      <input
        id="confirm"
        type="password"
        bind:value={confirm}
        autocomplete="new-password"
        aria-invalid={mismatch}
        required
      />
      {#if mismatch}
        <p class="hint" role="alert" style="color: var(--danger)">
          These don't match yet.
        </p>
      {/if}
    </div>

    <div class="alert warn">
      <label class="ack">
        <input type="checkbox" bind:checked={acknowledged} />
        <span>
          I understand that if I forget this passphrase, nobody can recover it —
          not even the people who made this app — and the files encrypted to my
          key will stay locked forever.
        </span>
      </label>
    </div>

    {#if error}
      <p class="alert error" role="alert">{error}</p>
    {/if}

    <div class="row">
      <button type="button" onclick={back}>Back</button>
      <div class="spacer"></div>
      <button class="primary" type="submit" disabled={!ready}>
        {busy ? 'Creating your key…' : 'Create my key'}
      </button>
    </div>
  </form>
{/if}
</main>

<style>
  .centered {
    height: 100%;
    display: grid;
    place-items: center;
    padding: 2rem;
    overflow-y: auto;
  }
  form, .card { max-width: 34rem; }
  h1 { margin: 0; font-size: 1.4rem; }
  .lede { margin: 0; color: var(--text-dim); }
  .big {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.15rem;
    padding: 0.8rem 1rem;
    text-align: left;
  }
  .sub { font-size: 0.85rem; opacity: 0.8; font-weight: 400; }
  code {
    font-family: ui-monospace, monospace;
    background: var(--surface-2);
    padding: 0 0.25rem;
    border-radius: 4px;
  }
  .ack {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
    font-weight: 400;
    margin: 0;
  }
  .ack input { margin-top: 0.25rem; flex-shrink: 0; }
</style>
