// Applies the theme preference to the document.
//
// Go owns the stored preference; this module owns the one line of DOM that
// makes it visible. See style.css for what the attribute means.

import type { AppSettings, Theme } from './api'

/**
 * Narrows the DTO's plain string to a Theme.
 *
 * Go validates the stored value, so an unrecognised one means the two sides
 * have drifted apart. Falling back to the desktop theme keeps the app readable
 * either way, which beats painting it from a value nothing understands.
 */
export function themeOf(settings: AppSettings): Theme {
  const t = settings.theme
  return t === 'light' || t === 'dark' ? t : 'system'
}

/**
 * Paints the app in the given theme.
 *
 * "system" removes the attribute rather than setting it, which hands the
 * decision back to the prefers-color-scheme rule in style.css. That keeps the
 * desktop-following logic in one place: CSS already tracks the desktop live, so
 * a user who flips their OS to dark while the app is open sees it follow
 * without any listener here.
 */
export function applyTheme(theme: Theme): void {
  const root = document.documentElement
  if (theme === 'system') {
    root.removeAttribute('data-theme')
  } else {
    root.setAttribute('data-theme', theme)
  }
}
