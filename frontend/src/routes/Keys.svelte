<script lang="ts">
  /** The user's own key: view, copy, export. */
  import { keys, message, type KeyStatus } from '../lib/api'

  let { status, announce }: { status: KeyStatus; announce: (m: string) => void } =
    $props()

  let toast = $state('')
  let error = $state('')

  const isPQ = $derived(status.keyType === 'hybrid-pq')

  async function copy() {
    error = ''
    try {
      await keys.copyPublicKey()
      toast = 'Public key copied to the clipboard.'
      announce(toast)
    } catch (e) {
      error = message(e)
      announce(error)
    }
  }

  async function exportKey() {
    error = ''
    try {
      const path = await keys.exportPublicKey()
      // "" means the user cancelled the dialog, which needs no feedback.
      if (!path) return
      toast = `Saved to ${path}`
      announce('Public key saved.')
    } catch (e) {
      error = message(e)
      announce(error)
    }
  }

  async function backup() {
    error = ''
    backupPath = ''
    try {
      const path = await keys.backup()
      if (!path) return // cancelled
      backupPath = path
      announce('Backup saved.')
    } catch (e) {
      error = message(e)
      announce(error)
    }
  }

  let backupPath = $state('')
</script>

<header>
  <h2>Your key</h2>
  <p class="lede">
    Share the key below with anyone who wants to send you a file. It's safe to
    post publicly.
  </p>
</header>

<div class="stack">
  <section class="card stack">
    <div class="row">
      <h3>Public key</h3>
      {#if isPQ}
        <span class="badge pq" title="X25519 + ML-KEM-768">Quantum-resistant</span>
      {:else}
        <span class="badge">Classic</span>
      {/if}
    </div>

    <!-- Long keys scroll inside their own box; the page never scrolls sideways. -->
    <div class="key" role="textbox" aria-readonly="true" aria-label="Your public key" tabindex="0">
      {status.publicKey}
    </div>

    <div class="row">
      <button class="primary" onclick={copy}>Copy</button>
      <button onclick={exportKey}>Save to a file…</button>
    </div>

    {#if toast}
      <p class="alert ok">{toast}</p>
    {/if}
    {#if error}
      <p class="alert error" role="alert">{error}</p>
    {/if}
  </section>

  <section class="card stack">
    <h3>Back up your key</h3>
    <p class="lede">
      If this computer is lost or wiped, a backup is the only way to read your
      files again. There is no reset — nobody can recover your key for you.
    </p>
    <p class="lede">
      The backup stays encrypted with the same passphrase, so it's safe to keep
      on a USB stick or in cloud storage. Anyone who finds it still needs your
      passphrase.
    </p>
    <div class="row">
      <button class="primary" onclick={backup}>Save a backup…</button>
    </div>
    {#if backupPath}
      <p class="alert ok">
        Backup saved to <code>{backupPath}</code>. Keep it somewhere you'll still
        have it if this computer doesn't work.
      </p>
    {/if}
  </section>

  <section class="card stack">
    <h3>Your private key</h3>
    <p class="lede">
      It stays on this computer, encrypted with your passphrase. It is never sent
      anywhere and never leaves this app.
    </p>
    <p class="alert warn">
      Never share your private key. Anyone who has it can read everything sent to
      you. A <strong>public</strong> key starts with <code>age1</code>; a private one
      starts with <code>AGE-SECRET-KEY</code>.
    </p>
  </section>
</div>

<style>
  header { margin-bottom: 1.2rem; }
  h2 { margin: 0 0 0.3rem; }
  h3 { margin: 0; font-size: 1rem; }
  .lede { margin: 0; color: var(--text-dim); }
  code {
    font-family: ui-monospace, monospace;
    background: var(--surface-2);
    padding: 0 0.25rem;
    border-radius: 4px;
  }
</style>
