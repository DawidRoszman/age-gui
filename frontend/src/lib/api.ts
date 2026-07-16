// Typed wrappers over the generated Wails bindings.
//
// The Go handlers return result envelopes ({ value, error }) rather than
// throwing, because Wails would flatten a Go error to a bare string and the UI
// needs to branch on the specific outcome. This module turns those envelopes
// back into ordinary throw/catch, so callers read naturally and still get the
// code.

import * as KeysGo from '../../wailsjs/go/view/Keys'
import * as ContactsGo from '../../wailsjs/go/view/Contacts'
import * as GroupsGo from '../../wailsjs/go/view/Groups'
import * as CryptoGo from '../../wailsjs/go/view/Crypto'
import * as SettingsGo from '../../wailsjs/go/view/Settings'
import { view } from '../../wailsjs/go/models'

/** Error codes, mirroring internal/view/errmap.go. These are contract. */
export const Code = {
  Locked: 'LOCKED',
  NoIdentity: 'NO_IDENTITY',
  IdentityExists: 'IDENTITY_EXISTS',
  WrongPassphrase: 'WRONG_PASSPHRASE',
  CorruptIdentity: 'CORRUPT_IDENTITY',
  NotForYou: 'NOT_FOR_YOU',
  PassphraseRequired: 'PASSPHRASE_REQUIRED',
  KeyRequired: 'KEY_REQUIRED',
  TargetExists: 'TARGET_EXISTS',
  DuplicateContact: 'DUPLICATE_CONTACT',
  ContactNotFound: 'CONTACT_NOT_FOUND',
  NoRecipients: 'NO_RECIPIENTS',
  SecretKeyGiven: 'SECRET_KEY_GIVEN',
  EmptyPassphrase: 'EMPTY_PASSPHRASE',
  InvalidSettings: 'INVALID_SETTINGS',
  NotAnIdentityFile: 'NOT_AN_IDENTITY_FILE',
  Cancelled: 'CANCELLED',
  Internal: 'INTERNAL',
} as const

export type ErrorCode = (typeof Code)[keyof typeof Code]

/** AppError carries the machine-readable code alongside the display message. */
export class AppError extends Error {
  readonly code: string
  readonly recoverable: boolean

  constructor(e: view.Error) {
    super(e.message)
    this.name = 'AppError'
    this.code = e.code
    this.recoverable = e.recoverable
  }
}

/** Throws if the envelope carries an error; otherwise returns the payload. */
function unwrap<T>(res: { error?: view.Error }, value: T): T {
  if (res.error) throw new AppError(res.error)
  return value
}

export type KeyStatus = view.KeyStatusDTO
export type Contact = view.ContactDTO
export type Group = view.GroupDTO

/**
 * Mirrors view.ProgressEvent in Go.
 *
 * Declared by hand because Wails only generates models for types reachable
 * from a bound method's signature, and this one travels over the event bus
 * instead. Keep in sync with internal/view/dto.go.
 */
export interface ProgressEvent {
  jobId: string
  done: number
  total: number
  percent: number
}

/** Event name emitted by the Go crypto handler. Matches view.EventProgress. */
export const EVENT_PROGRESS = 'crypto:progress'

/** Event name emitted by main.go when files are dropped on the window. */
export const EVENT_FILES_DROPPED = 'files:dropped'

/** Emitted when the key is dropped after an idle period. Matches view.EventAutoLocked. */
export const EVENT_AUTO_LOCKED = 'keys:auto-locked'

export type AppSettings = view.SettingsDTO

/** Themes, mirroring model.Theme in Go. "system" follows the desktop. */
export type Theme = 'system' | 'light' | 'dark'

export const keys = {
  async status(): Promise<KeyStatus> {
    const r = await KeysGo.Status()
    return unwrap(r, r.status)
  },
  async generate(passphrase: string): Promise<KeyStatus> {
    const r = await KeysGo.Generate(passphrase)
    return unwrap(r, r.status)
  },
  async unlock(passphrase: string): Promise<KeyStatus> {
    const r = await KeysGo.Unlock(passphrase)
    return unwrap(r, r.status)
  },
  async lock(): Promise<KeyStatus> {
    const r = await KeysGo.Lock()
    return unwrap(r, r.status)
  },
  async copyPublicKey(): Promise<void> {
    unwrap(await KeysGo.CopyPublicKey(), undefined)
  },
  /** Returns the saved path, or "" if the user cancelled the dialog. */
  async exportPublicKey(): Promise<string> {
    const r = await KeysGo.ExportPublicKey()
    return unwrap(r, r.value)
  },
  /** Saves an encrypted backup. Returns the path, or "" if cancelled. */
  async backup(): Promise<string> {
    const r = await KeysGo.Backup()
    return unwrap(r, r.value)
  },
  /**
   * Restores a key from a backup or an age-keygen file.
   * Returns null when the file dialog was cancelled.
   */
  async restore(passphrase: string): Promise<KeyStatus | null> {
    const r = await KeysGo.Restore(passphrase)
    const s = unwrap(r, r.status)
    return s.exists ? s : null
  },
  /** Resets the idle auto-lock countdown. Safe to call at any time. */
  touch(): Promise<void> {
    return KeysGo.Touch()
  },
}

export const settings = {
  async get(): Promise<AppSettings> {
    const r = await SettingsGo.Get()
    return unwrap(r, r.settings)
  },
  /** Pass 0 to turn auto-lock off. */
  async setAutoLock(minutes: number): Promise<AppSettings> {
    const r = await SettingsGo.SetAutoLock(minutes)
    return unwrap(r, r.settings)
  },
  /** Pass "" to go back to the downloads folder. */
  async setEncryptDir(dir: string): Promise<AppSettings> {
    const r = await SettingsGo.SetEncryptDir(dir)
    return unwrap(r, r.settings)
  },
  /** Pass "" to go back to the downloads folder. */
  async setDecryptDir(dir: string): Promise<AppSettings> {
    const r = await SettingsGo.SetDecryptDir(dir)
    return unwrap(r, r.settings)
  },
  /** Returns "" when the folder picker was cancelled. */
  async chooseDir(title: string, startDir: string): Promise<string> {
    const r = await SettingsGo.ChooseDir(title, startDir)
    return unwrap(r, r.value)
  },
  async setTheme(theme: Theme): Promise<AppSettings> {
    const r = await SettingsGo.SetTheme(theme)
    return unwrap(r, r.settings)
  },
}

export const contacts = {
  async list(): Promise<Contact[]> {
    const r = await ContactsGo.List()
    return unwrap(r, r.contacts)
  },
  async add(name: string, publicKey: string, note: string): Promise<Contact> {
    const r = await ContactsGo.Add(name, publicKey, note)
    return unwrap(r, r.contact)
  },
  /** Returns null when the file dialog was cancelled. */
  async importFromFile(name: string, note: string): Promise<Contact | null> {
    const r = await ContactsGo.ImportFromFile(name, note)
    const c = unwrap(r, r.contact)
    return c && c.id ? c : null
  },
  async rename(id: string, name: string, note: string): Promise<Contact> {
    const r = await ContactsGo.Rename(id, name, note)
    return unwrap(r, r.contact)
  },
  async remove(id: string): Promise<void> {
    unwrap(await ContactsGo.Delete(id), undefined)
  },
  async copyPublicKey(id: string): Promise<void> {
    unwrap(await ContactsGo.CopyPublicKey(id), undefined)
  },
}

export const groups = {
  async list(): Promise<Group[]> {
    const r = await GroupsGo.List()
    return unwrap(r, r.groups)
  },
  async create(name: string, memberIds: string[]): Promise<Group> {
    const r = await GroupsGo.Create(name, memberIds)
    return unwrap(r, r.group)
  },
  async update(id: string, name: string, memberIds: string[]): Promise<Group> {
    const r = await GroupsGo.Update(id, name, memberIds)
    return unwrap(r, r.group)
  },
  async remove(id: string): Promise<void> {
    unwrap(await GroupsGo.Delete(id), undefined)
  },
}

export type FileKind = 'passphrase' | 'recipients'

export const crypto = {
  async pickFiles(title: string): Promise<string[]> {
    const r = await CryptoGo.PickFiles(title)
    return unwrap(r, r.paths)
  },
  /** Returns "" when cancelled. */
  async chooseSavePath(title: string, defaultName: string): Promise<string> {
    const r = await CryptoGo.ChooseSavePath(title, defaultName)
    return unwrap(r, r.value)
  },
  async suggestEncryptOutput(input: string): Promise<string> {
    const r = await CryptoGo.SuggestEncryptOutput(input)
    return unwrap(r, r.value)
  },
  async suggestDecryptOutput(input: string): Promise<string> {
    const r = await CryptoGo.SuggestDecryptOutput(input)
    return unwrap(r, r.value)
  },
  async inspect(path: string): Promise<FileKind> {
    const r = await CryptoGo.Inspect(path)
    return unwrap(r, r.kind) as FileKind
  },
  async baseName(path: string): Promise<string> {
    const r = await CryptoGo.BaseName(path)
    return unwrap(r, r.value)
  },
  /** Opens the OS file manager showing `path`. */
  async showInFolder(path: string): Promise<void> {
    unwrap(await CryptoGo.ShowInFolder(path), undefined)
  },
  /**
   * `out` empty means the save folder from Settings, with a numbered name if
   * something is already there. Nothing is ever overwritten.
   *
   * `includeSelf` adds the user's own key, so they can open the file too.
   */
  async encrypt(
    jobId: string,
    input: string,
    out: string,
    contactIds: string[],
    includeSelf: boolean
  ): Promise<string> {
    const r = await CryptoGo.Encrypt(jobId, input, out, contactIds, includeSelf)
    return unwrap(r, r.value)
  },
  async encryptWithPassphrase(jobId: string, input: string, out: string, passphrase: string): Promise<string> {
    const r = await CryptoGo.EncryptWithPassphrase(jobId, input, out, passphrase)
    return unwrap(r, r.value)
  },
  async decrypt(jobId: string, input: string, out: string): Promise<string> {
    const r = await CryptoGo.Decrypt(jobId, input, out)
    return unwrap(r, r.value)
  },
  async decryptWithPassphrase(jobId: string, input: string, out: string, passphrase: string): Promise<string> {
    const r = await CryptoGo.DecryptWithPassphrase(jobId, input, out, passphrase)
    return unwrap(r, r.value)
  },
  async cancel(jobId: string): Promise<void> {
    unwrap(await CryptoGo.Cancel(jobId), undefined)
  },
}

/** Turns any thrown value into a message worth showing. */
export function message(e: unknown): string {
  if (e instanceof AppError) return e.message
  if (e instanceof Error) return e.message
  return String(e)
}

/** Reports whether a thrown value carries a specific code. */
export function hasCode(e: unknown, code: string): boolean {
  return e instanceof AppError && e.code === code
}

let jobCounter = 0
/** Unique id so progress events can be matched to the operation. */
export function newJobId(): string {
  jobCounter += 1
  return `job-${Date.now()}-${jobCounter}`
}
