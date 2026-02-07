import { useSyncExternalStore } from 'react'

type Theme = 'light' | 'dark'

const STORAGE_KEY = 'theme'

function getStoredTheme(): Theme | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored === 'light' || stored === 'dark') {
      return stored
    }
  } catch {
    // Storage access may be blocked in some privacy modes
  }
  return null
}

function getSystemTheme(): Theme {
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches
      ? 'dark'
      : 'light'
  }
  return 'light'
}

function resolveTheme(): Theme {
  return getStoredTheme() ?? getSystemTheme()
}

function applyTheme(theme: Theme): void {
  if (typeof document === 'undefined') return
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

let listeners: Array<() => void> = []

function emitChange(): void {
  for (const listener of listeners) {
    listener()
  }
}

function subscribe(listener: () => void): () => void {
  listeners = [...listeners, listener]
  return () => {
    listeners = listeners.filter((l) => l !== listener)
  }
}

function getSnapshot(): Theme {
  return resolveTheme()
}

function setTheme(theme: Theme): void {
  try {
    localStorage.setItem(STORAGE_KEY, theme)
  } catch {
    // Storage access may be blocked in some privacy modes
  }
  applyTheme(theme)
  emitChange()
}

function toggleTheme(): void {
  const current = resolveTheme()
  setTheme(current === 'dark' ? 'light' : 'dark')
}

// Listen to OS theme changes for when no explicit theme is stored
if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')

  const handleChange = () => {
    if (getStoredTheme() === null) {
      applyTheme(getSystemTheme())
      emitChange()
    }
  }

  if (typeof mediaQuery.addEventListener === 'function') {
    mediaQuery.addEventListener('change', handleChange)
  } else if (typeof (mediaQuery as any).addListener === 'function') {
    ;(mediaQuery as any).addListener(handleChange)
  }
}

export function useTheme(): { theme: Theme; toggleTheme: () => void } {
  const theme = useSyncExternalStore(subscribe, getSnapshot)
  return { theme, toggleTheme }
}

export const _testing = {
  resolveTheme,
  toggleTheme,
  applyTheme,
  getStoredTheme,
  getSystemTheme,
  subscribe,
}
