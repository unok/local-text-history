import { useSyncExternalStore } from 'react'

const listeners = new Set<() => void>()

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot() {
  return window.location.pathname + window.location.search
}

function notifyListeners() {
  for (const fn of listeners) fn()
}

window.addEventListener('popstate', notifyListeners)

export function navigate(path: string) {
  window.history.pushState(null, '', path)
  notifyListeners()
}

export function replaceUrl(path: string) {
  window.history.replaceState(null, '', path)
  notifyListeners()
}

interface DashboardRoute {
  page: 'dashboard'
  query: string
}

interface FileRoute {
  page: 'file'
  fileId: string
  fromId: string | null
  toId: string | null
}

export type Route = DashboardRoute | FileRoute

export function useRoute(): Route {
  const url = useSyncExternalStore(subscribe, getSnapshot)
  const urlObj = new URL(url, window.location.origin)
  const path = urlObj.pathname

  // /files/:fileId/diff/:fromId/:toId (explicit from/to)
  const explicitDiffMatch = path.match(
    /^\/files\/([^/]+)\/diff\/([^/]+)\/([^/]+)$/,
  )
  if (explicitDiffMatch) {
    return {
      page: 'file',
      fileId: explicitDiffMatch[1],
      fromId: explicitDiffMatch[2],
      toId: explicitDiffMatch[3],
    }
  }

  // /files/:fileId/diff/:toId (auto-resolve from to previous)
  const autoDiffMatch = path.match(/^\/files\/([^/]+)\/diff\/([^/]+)$/)
  if (autoDiffMatch) {
    return {
      page: 'file',
      fileId: autoDiffMatch[1],
      fromId: null,
      toId: autoDiffMatch[2],
    }
  }

  // /files/:fileId
  const fileMatch = path.match(/^\/files\/([^/]+)$/)
  if (fileMatch) {
    return { page: 'file', fileId: fileMatch[1], fromId: null, toId: null }
  }

  return { page: 'dashboard', query: urlObj.searchParams.get('q') ?? '' }
}
