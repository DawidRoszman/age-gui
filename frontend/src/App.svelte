<script lang="ts">
  import { onMount } from 'svelte'
  import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'
  import { keys, message, EVENT_AUTO_LOCKED, type KeyStatus } from './lib/api'
  import Onboarding from './routes/Onboarding.svelte'
  import Unlock from './routes/Unlock.svelte'
  import KeysView from './routes/Keys.svelte'
  import ContactsView from './routes/Contacts.svelte'
  import Encrypt from './routes/Encrypt.svelte'
  import Decrypt from './routes/Decrypt.svelte'
  import SettingsView from './routes/Settings.svelte'

  type Route = 'keys' | 'contacts' | 'encrypt' | 'decrypt' | 'settings'

  const NAV: { id: Route; label: string; key: string }[] = [
    { id: 'keys', label: 'Your key', key: '1' },
    { id: 'contacts', label: 'Contacts', key: '2' },
    { id: 'encrypt', label: 'Encrypt', key: '3' },
    { id: 'decrypt', label: 'Decrypt', key: '4' },
    { id: 'settings', label: 'Settings', key: '5' },
  ]

  // How often activity is reported to Go. The idle timeout is minutes, so
  // there is no point sending a call per keystroke — one every few seconds
  // resolves the countdown far more precisely than it needs.
  const TOUCH_INTERVAL_MS = 5000

  let status = $state<KeyStatus | null>(null)
  let route = $state<Route>('keys')
  let loadError = $state('')
  // Announced to screen readers. Without this, a keyboard-only user gets no
  // feedback at all from actions whose only result is a toast.
  let announcement = $state('')
  let lastTouch = 0

  function announce(msg: string) {
    // Clear first so repeating the same message is still announced.
    announcement = ''
    requestAnimationFrame(() => (announcement = msg))
  }

  async function refresh() {
    try {
      status = await keys.status()
      loadError = ''
    } catch (e) {
      loadError = message(e)
    }
  }

  onMount(() => {
    refresh()

    // Go owns the idle clock and the decision to lock; it just cannot see the
    // user. When it locks, move them to the unlock screen rather than leave
    // them on controls that have quietly stopped working.
    EventsOn(EVENT_AUTO_LOCKED, () => {
      if (status) status = { ...status, unlocked: false, publicKey: '', abbrev: '' }
      announce('Your key was locked after a period of inactivity.')
    })
    return () => EventsOff(EVENT_AUTO_LOCKED)
  })

  /**
   * Reports activity to Go, throttled.
   *
   * Only the UI can see the user, but Go decides when to lock — so a wedged or
   * compromised frontend can at worst fail to report activity (locking early),
   * never keep the key alive by lying.
   */
  function touch() {
    if (!status?.unlocked) return
    const now = Date.now()
    if (now - lastTouch < TOUCH_INTERVAL_MS) return
    lastTouch = now
    keys.touch().catch(() => {
      // Losing a heartbeat is not worth bothering the user about: the worst
      // case is the key locks sooner than they expected.
    })
  }

  async function lock() {
    try {
      status = await keys.lock()
      announce('Locked.')
    } catch (e) {
      announce(message(e))
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (!status?.unlocked) return
    // Never intercept a bare key while the user is typing.
    if (!e.ctrlKey && !e.metaKey) return
    // Deliberately not binding Ctrl+C: it must stay copy-the-selection.
    // Copying the key has its own focusable button.
    const hit = NAV.find((n) => n.key === e.key)
    if (hit) {
      e.preventDefault()
      route = hit.id
      return
    }
    if (e.key === 'l') {
      e.preventDefault()
      lock()
    }
  }
</script>

<!-- Activity for the idle countdown. Passive listeners so tracking a keystroke
     can never make typing feel slow. -->
<svelte:window
  onkeydown={onKeydown}
  onkeyup={touch}
  onpointerdown={touch}
  onpointermove={touch}
  onwheel={touch}
/>

<!-- Polite so it waits for the user to stop typing rather than interrupting. -->
<div class="sr-only" role="status" aria-live="polite">{announcement}</div>

{#if loadError}
  <main class="centered">
    <div class="card stack" style="max-width: 30rem">
      <h1>Age GUI</h1>
      <p class="alert error">{loadError}</p>
      <button class="primary" onclick={refresh}>Try again</button>
    </div>
  </main>
{:else if status === null}
  <main class="centered"><p>Loading…</p></main>
{:else if !status.exists}
  <Onboarding
    onDone={(s) => {
      status = s
      announce('Your key has been created.')
    }}
  />
{:else if !status.unlocked}
  <Unlock
    onDone={(s) => {
      status = s
      announce('Unlocked.')
    }}
  />
{:else}
  <div class="shell">
    <nav aria-label="Sections">
      <h1>Age GUI</h1>
      <ul>
        {#each NAV as item (item.id)}
          <li>
            <button
              class="navlink"
              class:active={route === item.id}
              aria-current={route === item.id ? 'page' : undefined}
              onclick={() => (route = item.id)}
            >
              <span>{item.label}</span>
              <kbd>Ctrl+{item.key}</kbd>
            </button>
          </li>
        {/each}
      </ul>
      <div class="spacer"></div>
      <button class="ghost lock" onclick={lock}>
        <span>Lock</span><kbd>Ctrl+L</kbd>
      </button>
    </nav>

    <main>
      {#if route === 'keys'}
        <KeysView {status} {announce} />
      {:else if route === 'contacts'}
        <ContactsView {announce} />
      {:else if route === 'encrypt'}
        <Encrypt {announce} />
      {:else if route === 'decrypt'}
        <Decrypt {announce} onLocked={refresh} />
      {:else}
        <SettingsView {announce} />
      {/if}
    </main>
  </div>
{/if}

<style>
  .centered {
    height: 100%;
    display: grid;
    place-items: center;
    padding: 2rem;
  }

  .shell {
    display: grid;
    grid-template-columns: 220px 1fr;
    height: 100%;
  }

  nav {
    background: var(--surface);
    border-right: 1px solid var(--border);
    padding: 1rem 0.7rem;
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  nav h1 {
    font-size: 1rem;
    margin: 0.2rem 0.4rem 0.8rem;
    color: var(--text-dim);
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 0.2rem; }

  .navlink, .lock {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    background: transparent;
    border-color: transparent;
    text-align: left;
  }
  .navlink:hover, .lock:hover { background: var(--surface-2); border-color: var(--border); }
  .navlink.active {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-text);
    font-weight: 600;
  }

  kbd {
    font: 0.7rem ui-monospace, monospace;
    color: var(--text-dim);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0 0.25rem;
  }
  .navlink.active kbd { color: inherit; border-color: currentColor; opacity: 0.7; }

  main {
    overflow-y: auto;
    padding: 1.5rem;
  }
</style>
