<script lang="ts">
  /** Preferences. Currently just idle auto-lock. */
  import { onMount } from 'svelte'
  import { settings, message, type AppSettings } from '../lib/api'

  let { announce }: { announce: (m: string) => void } = $props()

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
