import { useSyncExternalStore } from 'react'

interface TabState {
  query: string
  page: number
}

const defaultTabState: TabState = { query: '', page: 0 }

// Module-level state
let activeWatchSet: string | null = null
const tabStates = new Map<string, TabState>()
const listeners = new Set<() => void>()

// Snapshot counter for useSyncExternalStore change detection
let snapshotVersion = 0

// Incremented only on resetAllTabStates, used by consumers to detect resets
let resetVersion = 0

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot(): number {
  return snapshotVersion
}

function notifyListeners() {
  snapshotVersion++
  for (const fn of listeners) fn()
}

function setActiveWatchSet(name: string | null): void {
  if (activeWatchSet === name) return
  activeWatchSet = name
  notifyListeners()
}

function getTabState(name: string): TabState {
  return tabStates.get(name) ?? defaultTabState
}

function setTabState(name: string, state: Partial<TabState>): void {
  const current = tabStates.get(name) ?? { ...defaultTabState }
  const updated = { ...current, ...state }
  tabStates.set(name, updated)
  notifyListeners()
}

function resetAllTabStates(): void {
  tabStates.clear()
  resetVersion++
  notifyListeners()
}

export const _testing = {
  subscribe,
  getSnapshot,
  setActiveWatchSet,
  getTabState,
  setTabState,
  resetAllTabStates,
  getResetVersion: () => resetVersion,
  getActiveWatchSet: () => activeWatchSet,
  getTabStates: () => tabStates,
}

export function useWatchSetState() {
  useSyncExternalStore(subscribe, getSnapshot)

  const current = activeWatchSet
  const tabState = current !== null ? getTabState(current) : defaultTabState

  return {
    activeWatchSet: current,
    setActiveWatchSet,
    tabState,
    resetVersion,
    resetAllTabStates,
    setQuery: (query: string) => {
      if (current !== null) {
        setTabState(current, { query, page: 0 })
      }
    },
    setPage: (page: number) => {
      if (current !== null) {
        setTabState(current, { page })
      }
    },
  }
}
