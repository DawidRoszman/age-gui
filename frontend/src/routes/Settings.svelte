<script lang="ts">
  /** Preferences: where files are saved, and idle auto-lock. */
  import { onMount } from 'svelte'
  import { settings, message, type AppSettings } from '../lib/api'

  let { announce }: { announce: (m: string) => void } = $props()

  type Which = 'encrypt' | 'decrypt'

  // Offered periods. A short list of sensible choices beats a free-text minutes
  // box that invites the user to think about a number they do not care about.
  const CHOICES = [1, 5, 15, 30, 60, 240]

  let current = $state<AppSettings | null>(null)
  let error = $state('')
  let toast = $state('')
  let busy = $state(false)

  // What the radio group is bound to. Kept separate from `current` so the UI
  // does not flicker back while the save is in flight.
  let enabled = $state(true)
  let minutes = $state(15)

  onMount(async () => {
    try {
      const s = await settings.get()
      current = s
      enabled = s.autoLockEnabled
      minutes = s.autoLockEnabled ? s.autoLockMinutes : 15
    } catch (e) {
      error = message(e)
    }
  })

  async function save(nextEnabled: boolean, nextMinutes: number) {
    busy = true
    error = ''
    toast = ''
    try {
      // 0 is the "off" sentinel the Go side understands.
      const s = await settings.setAutoLock(nextEnabled ? nextMinutes : 0)
      current = s
      enabled = s.autoLockEnabled
      if (s.autoLockEnabled) minutes = s.autoLockMinutes
      toast = s.autoLockEnabled
        ? `Your key will lock after ${describe(s.autoLockMinutes)} of inactivity.`
        : 'Auto-lock is off. Your key stays unlocked until you lock it or quit.'
      announce(toast)
    } catch (e) {
      error = message(e)
      // Snap the control back to what is actually in force, so it never lies.
      if (current) {
        enabled = current.autoLockEnabled
        if (current.autoLockEnabled) minutes = current.autoLockMinutes
      }
      announce(error)
    } finally {
      busy = false
    }
  }

  function describe(m: number): string {
    if (m < 60) return `${m} minute${m === 1 ? '' : 's'}`
    const h = m / 60
    return `${h} hour${h === 1 ? '' : 's'}`
  }

  function onToggle(next: boolean) {
    enabled = next
    save(next, minutes)
  }

  function onChoose(m: number) {
    minutes = m
    save(true, m)
  }

  // Folder rows below. The picker and the save are two calls on purpose: a
  // cancelled dialog and "reset to default" both come back as "", and treating
  // a cancel as a reset would silently undo the user's choice.
  async function changeDir(which: Which) {
    error = ''
    toast = ''
    const from = which === 'encrypt' ? current?.encryptDir : current?.decryptDir
    try {
      const chosen = await settings.chooseDir(
        which === 'encrypt' ? 'Where to save encrypted files' : 'Where to save decrypted files',
        from ?? ''
      )
      if (!chosen) return // cancelled
      await applyDir(which, chosen)
    } catch (e) {
      error = message(e)
      announce(error)
    }
  }

  async function resetDir(which: Which) {
    error = ''
    toast = ''
    try {
      await applyDir(which, '')
    } catch (e) {
      error = message(e)
      announce(error)
    }
  }

  async function applyDir(which: Which, dir: string) {
    busy = true
    try {
      current =
        which === 'encrypt'
          ? await settings.setEncryptDir(dir)
          : await settings.setDecryptDir(dir)
      const where = which === 'encrypt' ? current.encryptDir : current.decryptDir
      toast = `${which === 'encrypt' ? 'Encrypted' : 'Decrypted'} files will be saved to ${where}.`
      announce(toast)
    } finally {
      busy = false
    }
  }

</script>

<header>
  <h2>Settings</h2>
</header>

{#if error}
  <p class="alert error" role="alert">{error}</p>
{/if}

{#if current === null}
  <p>Loading…</p>
{:else}
  <section class="card stack">
    <h3>Where files are saved</h3>
    <p class="lede">
      New files go here, named after the file you started with. If that name is
      already taken, a number is added — nothing is ever replaced.
    </p>

    <div class="dir">
      <span class="dir-label" id="encrypt-dir-label">Encrypted files</span>
      <code class="path" aria-labelledby="encrypt-dir-label">{current.encryptDir}</code>
      <span class="dir-actions">
        {#if current.encryptDirIsDefault}
          <span class="badge">Downloads</span>
        {/if}
        <button class="ghost" disabled={busy} onclick={() => changeDir('encrypt')}>
          Change…
        </button>
        {#if !current.encryptDirIsDefault}
          <button class="ghost" disabled={busy} onclick={() => resetDir('encrypt')}>
            Use Downloads
          </button>
        {/if}
      </span>
    </div>

    <div class="dir">
      <span class="dir-label" id="decrypt-dir-label">Decrypted files</span>
      <code class="path" aria-labelledby="decrypt-dir-label">{current.decryptDir}</code>
      <span class="dir-actions">
        {#if current.decryptDirIsDefault}
          <span class="badge">Downloads</span>
        {/if}
        <button class="ghost" disabled={busy} onclick={() => changeDir('decrypt')}>
          Change…
        </button>
        {#if !current.decryptDirIsDefault}
          <button class="ghost" disabled={busy} onclick={() => resetDir('decrypt')}>
            Use Downloads
          </button>
        {/if}
      </span>
    </div>

    <p class="alert warn">
      A decrypted file is the plaintext of a secret. Somewhere only you can read
      is a better home for it than a folder you sync or share.
    </p>
  </section>

  <section class="card stack">
    <h3>Lock automatically when idle</h3>
    <p class="lede">
      While unlocked, your private key is held in memory. Locking after a period
      of inactivity means walking away from your computer doesn't leave it open.
      You'll type your passphrase again to carry on.
    </p>

    <fieldset>
      <legend class="sr-only">Auto-lock</legend>
      <label class="choice">
        <input
          type="radio"
          name="autolock"
          checked={enabled}
          disabled={busy}
          onchange={() => onToggle(true)}
        />
        <span>Lock after a period of inactivity</span>
      </label>
      <label class="choice">
        <input
          type="radio"
          name="autolock"
          checked={!enabled}
          disabled={busy}
          onchange={() => onToggle(false)}
        />
        <span>Never lock automatically</span>
      </label>
    </fieldset>

    {#if enabled}
      <fieldset class="periods">
        <legend>Lock after</legend>
        {#each CHOICES as m (m)}
          <label class="choice">
            <input
              type="radio"
              name="minutes"
              checked={minutes === m}
              disabled={busy}
              onchange={() => onChoose(m)}
            />
            <span>{describe(m)}</span>
          </label>
        {/each}
      </fieldset>
    {:else}
      <p class="alert warn">
        Your key will stay unlocked until you lock it yourself (Ctrl+L) or quit
        the app. Fine on a computer only you use; worth reconsidering on a shared
        or portable one.
      </p>
    {/if}

    {#if toast}
      <p class="alert ok">{toast}</p>
    {/if}
  </section>
{/if}

<style>
  header { margin-bottom: 1rem; }
  h2 { margin: 0; }
  h3 { margin: 0; font-size: 1rem; }
  .lede { margin: 0; color: var(--text-dim); }

  .dir {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
  .dir-label {
    font-weight: 600;
    min-width: 9ch;
  }
  /* A path is the one thing here the user must be able to read exactly, so it
     gets the room: it grows, and wraps rather than being cut off with an
     ellipsis that would hide which folder this actually is. */
  .path {
    flex: 1 1 20ch;
    min-width: 0;
    overflow-wrap: anywhere;
    background: var(--surface-2, rgba(127, 127, 127, 0.12));
    border-radius: 4px;
    padding: 0.25rem 0.45rem;
    font-size: 0.85rem;
  }
  .dir-actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex-shrink: 0;
  }
  .badge {
    font-size: 0.75rem;
    color: var(--text-dim);
    border: 1px solid var(--border, rgba(127, 127, 127, 0.4));
    border-radius: 999px;
    padding: 0.05rem 0.5rem;
  }
  fieldset { border: 0; padding: 0; margin: 0; }
  legend { font-weight: 600; padding: 0; margin-bottom: 0.4rem; }
  .periods { margin-top: 0.4rem; }
  .choice {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-weight: 400;
    margin: 0 0 0.3rem;
    cursor: pointer;
  }
</style>
