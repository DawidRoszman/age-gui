<script lang="ts">
  /** Unlock the stored key for this session. */
  import { keys, message, hasCode, Code, type KeyStatus } from '../lib/api'

  let { onDone }: { onDone: (s: KeyStatus) => void } = $props()

  let passphrase = $state('')
  let busy = $state(false)
  let error = $state('')
  let damaged = $state(false)
  let input: HTMLInputElement | undefined = $state()

  // The only thing to do on this screen is type the passphrase, so put the
  // cursor there rather than making a keyboard user tab to it.
  $effect(() => input?.focus())

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (busy || !passphrase) return
    busy = true
    error = ''
    try {
      const status = await keys.unlock(passphrase)
      passphrase = ''
      onDone(status)
    } catch (err) {
      error = message(err)
      // A damaged key file is a different situation from a typo, and the user
      // should reach for a backup rather than keep guessing.
      damaged = hasCode(err, Code.CorruptIdentity)
      if (!damaged) {
        passphrase = ''
        input?.focus()
      }
    } finally {
      busy = false
    }
  }
</script>

<main class="centered">
  <form class="card stack" onsubmit={submit}>
    <h1>Welcome back</h1>
    <p class="lede">Enter your passphrase to unlock your key.</p>

    <div class="field">
      <label for="pass">Passphrase</label>
      <input
        id="pass"
        type="password"
        bind:this={input}
        bind:value={passphrase}
        autocomplete="current-password"
        required
      />
    </div>

    {#if error}
      <p class="alert" class:error={!damaged} class:warn={damaged} role="alert">
        {error}
      </p>
    {/if}

    <button class="primary" type="submit" disabled={busy || !passphrase}>
      {busy ? 'Unlocking…' : 'Unlock'}
    </button>
  </form>
</main>

<style>
  .centered {
    height: 100%;
    display: grid;
    place-items: center;
    padding: 2rem;
  }
  form { max-width: 24rem; width: 100%; }
  h1 { margin: 0; font-size: 1.3rem; }
  .lede { margin: 0; color: var(--text-dim); }
</style>
