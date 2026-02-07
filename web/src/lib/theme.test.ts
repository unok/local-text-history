import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('theme', () => {
  const storage = new Map<string, string>()
  let mockMatchMediaMatches = false
  let mockMediaQueryListeners: Array<(event: { matches: boolean }) => void> = []
  let classList: Set<string>

  function createMatchMediaMock() {
    return vi.fn((query: string) => ({
      matches: mockMatchMediaMatches,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(
        (_: string, cb: (event: { matches: boolean }) => void) => {
          mockMediaQueryListeners.push(cb)
        },
      ),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
  }

  beforeEach(() => {
    storage.clear()
    mockMatchMediaMatches = false
    mockMediaQueryListeners = []
    classList = new Set()

    const matchMediaMock = createMatchMediaMock()

    vi.stubGlobal('localStorage', {
      getItem: vi.fn((key: string) => storage.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => storage.set(key, value)),
      removeItem: vi.fn((key: string) => storage.delete(key)),
      clear: vi.fn(() => storage.clear()),
    })

    vi.stubGlobal('document', {
      documentElement: {
        classList: {
          add: vi.fn((cls: string) => classList.add(cls)),
          remove: vi.fn((cls: string) => classList.delete(cls)),
          contains: (cls: string) => classList.has(cls),
        },
      },
    })

    vi.stubGlobal('matchMedia', matchMediaMock)

    // Ensure window exists and has matchMedia for non-browser guard checks
    vi.stubGlobal('window', {
      ...globalThis,
      matchMedia: matchMediaMock,
      localStorage: globalThis.localStorage,
    })
  })

  async function loadThemeModule() {
    vi.resetModules()
    return await import('./theme')
  }

  describe('resolveTheme', () => {
    it('returns light when no stored theme and system is light', async () => {
      mockMatchMediaMatches = false
      const { _testing } = await loadThemeModule()
      expect(_testing.resolveTheme()).toBe('light')
    })

    it('returns dark when stored theme is dark', async () => {
      storage.set('theme', 'dark')
      const { _testing } = await loadThemeModule()
      expect(_testing.resolveTheme()).toBe('dark')
    })

    it('returns dark when no stored theme and system prefers dark', async () => {
      mockMatchMediaMatches = true
      const { _testing } = await loadThemeModule()
      expect(_testing.resolveTheme()).toBe('dark')
    })

    it('returns light when stored theme is light and system prefers dark', async () => {
      storage.set('theme', 'light')
      mockMatchMediaMatches = true
      const { _testing } = await loadThemeModule()
      expect(_testing.resolveTheme()).toBe('light')
    })

    it('ignores invalid stored theme values and falls back to system', async () => {
      storage.set('theme', 'invalid')
      mockMatchMediaMatches = false
      const { _testing } = await loadThemeModule()
      expect(_testing.resolveTheme()).toBe('light')
    })
  })

  describe('toggleTheme', () => {
    it('toggles from light to dark and updates localStorage and DOM', async () => {
      mockMatchMediaMatches = false
      const { _testing } = await loadThemeModule()

      _testing.toggleTheme()
      expect(storage.get('theme')).toBe('dark')
      expect(classList.has('dark')).toBe(true)
    })

    it('toggles from dark to light and updates localStorage and DOM', async () => {
      storage.set('theme', 'dark')
      const { _testing } = await loadThemeModule()

      _testing.toggleTheme()
      expect(storage.get('theme')).toBe('light')
      expect(classList.has('dark')).toBe(false)
    })

    it('toggles multiple times correctly', async () => {
      mockMatchMediaMatches = false
      const { _testing } = await loadThemeModule()

      _testing.toggleTheme() // light → dark
      expect(classList.has('dark')).toBe(true)
      expect(storage.get('theme')).toBe('dark')

      _testing.toggleTheme() // dark → light
      expect(classList.has('dark')).toBe(false)
      expect(storage.get('theme')).toBe('light')

      _testing.toggleTheme() // light → dark
      expect(classList.has('dark')).toBe(true)
      expect(storage.get('theme')).toBe('dark')
    })
  })

  describe('applyTheme', () => {
    it('adds dark class for dark theme', async () => {
      const { _testing } = await loadThemeModule()
      _testing.applyTheme('dark')
      expect(classList.has('dark')).toBe(true)
    })

    it('removes dark class for light theme', async () => {
      classList.add('dark')
      const { _testing } = await loadThemeModule()
      _testing.applyTheme('light')
      expect(classList.has('dark')).toBe(false)
    })
  })

  describe('OS theme change listener', () => {
    it('registers listener for OS theme changes', async () => {
      await loadThemeModule()
      expect(mockMediaQueryListeners.length).toBeGreaterThan(0)
    })

    it('applies system theme on OS change when no stored theme', async () => {
      mockMatchMediaMatches = false
      await loadThemeModule()

      // Simulate OS switching to dark
      mockMatchMediaMatches = true
      for (const listener of mockMediaQueryListeners) {
        listener({ matches: true })
      }
      expect(classList.has('dark')).toBe(true)
    })

    it('ignores OS change when stored theme exists', async () => {
      storage.set('theme', 'light')
      await loadThemeModule()

      // Simulate OS switching to dark
      mockMatchMediaMatches = true
      for (const listener of mockMediaQueryListeners) {
        listener({ matches: true })
      }
      // Should remain light because stored theme takes precedence
      expect(classList.has('dark')).toBe(false)
    })
  })

  describe('localStorage error handling', () => {
    it('falls back to system theme when localStorage.getItem throws', async () => {
      mockMatchMediaMatches = true
      vi.stubGlobal('localStorage', {
        getItem: vi.fn(() => {
          throw new Error('Access denied')
        }),
        setItem: vi.fn((key: string, value: string) =>
          storage.set(key, value),
        ),
      })

      const { _testing } = await loadThemeModule()
      // getStoredTheme returns null on error, falls back to system (dark)
      expect(_testing.resolveTheme()).toBe('dark')
    })

    it('does not crash when localStorage.setItem throws', async () => {
      mockMatchMediaMatches = true
      vi.stubGlobal('localStorage', {
        getItem: vi.fn(() => null),
        setItem: vi.fn(() => {
          throw new Error('Access denied')
        }),
      })

      const { _testing } = await loadThemeModule()
      // Should not throw even though setItem fails
      expect(() => _testing.toggleTheme()).not.toThrow()
      // DOM should still be updated
      expect(classList.has('dark')).toBe(false)
    })
  })

  describe('subscribe', () => {
    it('notifies listeners on theme change', async () => {
      mockMatchMediaMatches = false
      const { _testing } = await loadThemeModule()

      const listener = vi.fn()
      const unsubscribe = _testing.subscribe(listener)

      _testing.toggleTheme()
      expect(listener).toHaveBeenCalledTimes(1)

      unsubscribe()
      _testing.toggleTheme()
      expect(listener).toHaveBeenCalledTimes(1) // Not called again
    })
  })
})
