# Age GUI

A friendly desktop wrapper around [age](https://age-encryption.org), so people
who don't use a terminal can still share secrets securely.

Create a keypair, share your public key, save your contacts' public keys, and
encrypt or decrypt files — no command line required.

## Status

Working end to end on Linux: the desktop binary builds, launches, and the core
is covered by tests including interop against the real age CLI. Windows and
macOS builds are wired into CI but have not been run on real hardware yet.

Stack: Go 1.26 + [Wails v2](https://wails.io) + Svelte 5 + TypeScript, with
[`filippo.io/age`](https://filippo.io/age) v1.3.1 doing the cryptography.

## Design

Layered strictly inward: **View → Service → Model**.

| Layer | Package | Responsibility |
|---|---|---|
| Model | `internal/model` | Pure domain: validated keys, contacts, domain errors. No I/O. |
| Service | `internal/service` | Use cases: key lifecycle, contacts, encrypt/decrypt. Declares its own storage ports. |
| Storage | `internal/storage` | Implements those ports over the filesystem. |
| View | `internal/view`, `frontend/` | Wails handlers + Svelte UI. The only layer that knows Wails exists. |

`model` and `service` have no GUI dependency at all and are tested without one.

### Key decisions

- **Post-quantum by default.** New keypairs are X25519 + ML-KEM-768 hybrid
  (`age1pq1…`), which age itself now calls the standard key type. The ~2000
  character key is irrelevant behind Copy and Export buttons.
- **Strict on generation, liberal on import.** We only ever *generate* hybrid
  keys, but a contact's key belongs to them — classic `age1…` keys are always
  accepted. Rejecting them would cut users off from most of the age ecosystem.
- **The key file is a plain age file.** `identity.age` is an armored,
  scrypt-encrypted age file. `age -d identity.age` opens it with the stock CLI,
  so your key is never trapped in this app.
- **File bytes never enter the webview.** Wails hands Go absolute *paths* from
  drag-drop and file dialogs, so crypto streams entirely in Go. Multi-gigabyte
  files work, and plaintext never crosses the JS bridge.
- **Nothing is ever overwritten silently**, and every write is atomic
  (temp + fsync + rename). A crash mid-write must never destroy the only copy of
  a private key.

### Where things live

`os.UserConfigDir()`, so: `~/.config/age-gui` on Linux, `%AppData%\age-gui` on
Windows, `~/Library/Application Support/age-gui` on macOS.

    identity.age    your keypair, encrypted with your passphrase (mode 0600)
    contacts.json   public keys only — no secrets
    settings.json   preferences only — no secrets

## Backing up your key

**`identity.age` is the only copy of your key.** Lose it with no backup and every
file encrypted to you is gone; there is no reset and nobody can recover it.

*Your key → Back up your key* writes the encrypted file wherever you choose. The
backup is the same ciphertext, so it is safe on a USB stick or in cloud storage —
anyone who finds it still needs your passphrase.

To restore on a new machine, pick **"I already have a key"** on the welcome
screen. That path also imports a plaintext key from `age-keygen -o key.txt`, so
an existing age CLI user can move in; the passphrase you give at import becomes
its protection, since the file arrives with none.

## Auto-lock

While unlocked, the private key is held in memory. By default it is dropped
after **15 minutes** of inactivity; *Settings* offers 1 minute to 4 hours, or
**Never**.

Disabling is a real, supported choice — someone encrypting a long batch of files
should not be interrupted, and a tool that fights its user gets replaced by one
that does not.

Go owns the clock and the decision; the frontend only reports activity, because
it is the only layer that can see the user. That direction matters: a wedged
frontend can at worst fail to report activity and lock *early*, never keep the
key alive by lying. A corrupt `settings.json` also falls back to the defaults —
which means auto-lock ends up **on**, the safe direction.

## Building

### Linux (Fedora)

    sudo dnf install gtk3-devel webkit2gtk4.1-devel
    go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
    wails dev -tags webkit2_41      # live reload
    wails build -tags webkit2_41    # release

### The `webkit2_41` tag (Fedora)

**Mandatory on Fedora, not optional.** Wails v2 links `webkit2gtk-4.0` by
default, and Fedora 40+ ships only 4.1 — there is no `webkit2gtk4.0-devel` to
fall back to. Without the tag the build dies in pkg-config:

    Package 'webkit2gtk-4.0' not found

The cgo directives live in `internal/frontend/desktop/linux/webkit2.go` in the
Wails module:

    #cgo !webkit2_41 pkg-config: webkit2gtk-4.0
    #cgo webkit2_41  pkg-config: webkit2gtk-4.1

Two traps worth knowing:

1. **`go build ./...` succeeds without the tag, and proves nothing.** Wails gates
   the GTK/WebKit frontend behind `//go:build !dev && !production`, so a plain
   build compiles a *stub* and never invokes pkg-config. Only `-tags production`
   (what `wails build` passes) exercises the real path. This is a feature for us
   — `go test ./...` needs no GUI libraries at all — but it means a green
   `go build` says nothing about whether the app links.
2. **`wails doctor` reports `libwebkit … Not Found` on Fedora even when
   everything works.** It probes for the 4.0 package name. Ignore it; build with
   the tag. It also mis-reports npm. (See wails#4457.)

**Ubuntu 24.04+ is in the same position** — it dropped `libwebkit2gtk-4.0-dev`
and ships only 4.1 — so use `libwebkit2gtk-4.1-dev` and the same tag there:

    sudo apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev
    wails build -tags webkit2_41

Only older distros that still carry `libwebkit2gtk-4.0-dev` can omit it.

### Cross-platform

Wails uses cgo, which does not cross-compile cleanly. Build each OS on its own
runner (the CI matrix does this); do not try Linux → Windows.

## Testing

    go test ./...                          # everything; no GUI or display needed
    cd frontend && npm run check           # svelte-check + TypeScript

Every Go package is testable without a display, a window, or the GTK headers —
see the build-tag note above for why. The `view` handlers manage it by depending
on a small `Platform` port (clipboard, dialogs, events) that tests fake out.

### Interop tests

`internal/service/interop_test.go` runs against the **real age CLI** rather than
only round-tripping through our own library — self-consistency proves nothing
about whether a user can actually exchange files with someone running stock age.
It verifies, for both post-quantum and classic keys:

- files we encrypt decrypt with `age -d -i`
- files `age` encrypts (binary *and* armored) decrypt in this app
- our exported public key works with `age -R`
- `age-keygen -y` output imports as a contact

These skip automatically when `age` and `age-keygen` are not in `PATH`.
Post-quantum cases need age ≥ 1.3.0 and skip on older versions.

### The one check that can't be automated

The age CLI reads passphrases from `/dev/tty` and refuses a pipe (*"standard
input is not a terminal"*), so passphrase flows can't be driven from a test
without a pty. Verify by hand that your key is not locked in:

    age -d ~/.config/age-gui/identity.age

It should prompt for your passphrase and print your `AGE-SECRET-KEY-PQ-1…`.

## Security notes

- The private key is decrypted only in memory, only while unlocked, and never
  crosses into the webview. DTOs carry public keys only.
- **Passphrases reach Go as strings.** This is inherent to any Wails/Electron
  style app: the value arrives over the JS bridge and Go strings are immutable,
  so copies may persist until garbage collection. Mitigated by never logging
  them and dropping the JS-side value immediately — but stated plainly rather
  than papered over.
- Contacts hold public keys only, so `contacts.json` carries no secrets.

### Scrypt work factor

Identity files and passphrase-encrypted files use age's default work factor
(logN=18), the same as the age CLI. Tests turn it down via an **unexported**
option — unexported precisely so no production path can reach it. Because a
weakened work factor would leave every other test passing happily,
`workfactor_test.go` asserts the production default explicitly and proves the
knob works, so those guards cannot pass vacuously.
