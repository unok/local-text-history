import { describe, it, expect, vi } from 'vitest'

describe('watchSetState', () => {
  async function loadModule() {
    vi.resetModules()
    return await import('./watchSetState')
  }

  describe('setActiveWatchSet', () => {
    it('should set the active watch set', async () => {
      const { _testing } = await loadModule()

      _testing.setActiveWatchSet('ws-1')
      expect(_testing.getActiveWatchSet()).toBe('ws-1')
    })

    it('should not notify listeners when setting same value', async () => {
      const { _testing } = await loadModule()
      const listener = vi.fn()
      _testing.subscribe(listener)

      _testing.setActiveWatchSet('ws-1')
      expect(listener).toHaveBeenCalledTimes(1)

      _testing.setActiveWatchSet('ws-1')
      expect(listener).toHaveBeenCalledTimes(1)
    })

    it('should notify listeners when value changes', async () => {
      const { _testing } = await loadModule()
      const listener = vi.fn()
      _testing.subscribe(listener)

      _testing.setActiveWatchSet('ws-1')
      _testing.setActiveWatchSet('ws-2')
      expect(listener).toHaveBeenCalledTimes(2)
    })

    it('should accept null to clear active watch set', async () => {
      const { _testing } = await loadModule()

      _testing.setActiveWatchSet('ws-1')
      _testing.setActiveWatchSet(null)
      expect(_testing.getActiveWatchSet()).toBeNull()
    })
  })

  describe('getTabState', () => {
    it('should return default state for unknown tab', async () => {
      const { _testing } = await loadModule()

      const state = _testing.getTabState('unknown')
      expect(state).toEqual({ query: '', page: 0 })
    })

    it('should return stored state for known tab', async () => {
      const { _testing } = await loadModule()

      _testing.setTabState('ws-1', { query: 'test', page: 3 })
      const state = _testing.getTabState('ws-1')
      expect(state).toEqual({ query: 'test', page: 3 })
    })
  })

  describe('setTabState', () => {
    it('should update partial state', async () => {
      const { _testing } = await loadModule()

      _testing.setTabState('ws-1', { query: 'hello' })
      expect(_testing.getTabState('ws-1')).toEqual({ query: 'hello', page: 0 })

      _testing.setTabState('ws-1', { page: 5 })
      expect(_testing.getTabState('ws-1')).toEqual({ query: 'hello', page: 5 })
    })

    it('should notify listeners on state change', async () => {
      const { _testing } = await loadModule()
      const listener = vi.fn()
      _testing.subscribe(listener)

      _testing.setTabState('ws-1', { query: 'search' })
      expect(listener).toHaveBeenCalledTimes(1)
    })

    it('should maintain independent state per tab', async () => {
      const { _testing } = await loadModule()

      _testing.setTabState('ws-1', { query: 'first', page: 1 })
      _testing.setTabState('ws-2', { query: 'second', page: 2 })

      expect(_testing.getTabState('ws-1')).toEqual({ query: 'first', page: 1 })
      expect(_testing.getTabState('ws-2')).toEqual({ query: 'second', page: 2 })
    })
  })

  describe('resetAllTabStates', () => {
    it('should clear all tab states', async () => {
      const { _testing } = await loadModule()

      _testing.setTabState('ws-1', { query: 'a', page: 1 })
      _testing.setTabState('ws-2', { query: 'b', page: 2 })

      _testing.resetAllTabStates()

      expect(_testing.getTabState('ws-1')).toEqual({ query: '', page: 0 })
      expect(_testing.getTabState('ws-2')).toEqual({ query: '', page: 0 })
      expect(_testing.getTabStates().size).toBe(0)
    })

    it('should increment reset version', async () => {
      const { _testing } = await loadModule()

      const initialVersion = _testing.getResetVersion()
      _testing.resetAllTabStates()
      expect(_testing.getResetVersion()).toBe(initialVersion + 1)

      _testing.resetAllTabStates()
      expect(_testing.getResetVersion()).toBe(initialVersion + 2)
    })

    it('should notify listeners', async () => {
      const { _testing } = await loadModule()
      const listener = vi.fn()
      _testing.subscribe(listener)

      _testing.resetAllTabStates()
      expect(listener).toHaveBeenCalledTimes(1)
    })

    it('should not increment reset version on other state changes', async () => {
      const { _testing } = await loadModule()

      const initialVersion = _testing.getResetVersion()
      _testing.setActiveWatchSet('ws-1')
      _testing.setTabState('ws-1', { query: 'test' })

      expect(_testing.getResetVersion()).toBe(initialVersion)
    })
  })

  describe('subscribe', () => {
    it('should register and unregister listeners', async () => {
      const { _testing } = await loadModule()
      const listener = vi.fn()

      const unsubscribe = _testing.subscribe(listener)
      _testing.setActiveWatchSet('ws-1')
      expect(listener).toHaveBeenCalledTimes(1)

      unsubscribe()
      _testing.setActiveWatchSet('ws-2')
      expect(listener).toHaveBeenCalledTimes(1)
    })

    it('should support multiple listeners', async () => {
      const { _testing } = await loadModule()
      const listener1 = vi.fn()
      const listener2 = vi.fn()

      _testing.subscribe(listener1)
      _testing.subscribe(listener2)

      _testing.setActiveWatchSet('ws-1')
      expect(listener1).toHaveBeenCalledTimes(1)
      expect(listener2).toHaveBeenCalledTimes(1)
    })
  })

  describe('getSnapshot', () => {
    it('should increment on state changes', async () => {
      const { _testing } = await loadModule()

      const v1 = _testing.getSnapshot()
      _testing.setActiveWatchSet('ws-1')
      const v2 = _testing.getSnapshot()
      expect(v2).toBeGreaterThan(v1)

      _testing.setTabState('ws-1', { page: 1 })
      const v3 = _testing.getSnapshot()
      expect(v3).toBeGreaterThan(v2)

      _testing.resetAllTabStates()
      const v4 = _testing.getSnapshot()
      expect(v4).toBeGreaterThan(v3)
    })
  })
})
