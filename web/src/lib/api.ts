import { useEffect, useCallback } from 'react'
import {
  useQuery,
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'

// Types matching Go server responses

export interface FileRecord {
  id: string
  path: string
  created: number
  updated: number
}

export interface Snapshot {
  id: string
  fileId: string
  size: number
  hash: string
  timestamp: number
}

export interface DiffResult {
  diff: string
  from: string
  to: string
}

export interface Stats {
  totalFiles: number
  totalSnapshots: number
  totalSize: number
  watchDirs: string[]
}

export interface HistoryEntry {
  snapshotId: string
  fileId: string
  filePath: string
  size: number
  hash: string
  timestamp: number
}

export interface RenameRecord {
  id: string
  oldFileId: string
  newFileId: string
  oldPath: string
  newPath: string
  timestamp: number
}

// API client

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error)
  }
  return res.json()
}

async function deleteRequest(url: string): Promise<void> {
  const res = await fetch(url, { method: 'DELETE' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error)
  }
}

// React Query hooks

export function useSearchFiles(query: string, limit: number, offset: number) {
  return useQuery({
    queryKey: ['files', query, limit, offset],
    queryFn: () =>
      fetchJSON<FileRecord[]>(
        `/api/files?q=${encodeURIComponent(query)}&limit=${limit}&offset=${offset}`,
      ),
  })
}

export function useSnapshots(fileId: string | null) {
  return useQuery({
    queryKey: ['snapshots', fileId],
    queryFn: () => fetchJSON<Snapshot[]>(`/api/files/${fileId}/snapshots`),
    enabled: fileId !== null,
  })
}

export function useDiff(fromId: string | null, toId: string | null) {
  return useQuery({
    queryKey: ['diff', fromId, toId],
    queryFn: () => {
      const params = new URLSearchParams({ to: toId! })
      if (fromId !== null) {
        params.set('from', fromId)
      }
      return fetchJSON<DiffResult>(`/api/diff?${params.toString()}`)
    },
    enabled: toId !== null,
  })
}

export function useStats() {
  return useQuery({
    queryKey: ['stats'],
    queryFn: () => fetchJSON<Stats>('/api/stats'),
  })
}

export function useDeleteFile() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteRequest(`/api/files/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['files'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
    },
  })
}

export function useFile(id: string) {
  return useQuery({
    queryKey: ['file', id],
    queryFn: () => fetchJSON<FileRecord>(`/api/files/${id}`),
  })
}

export function useRenames(fileId: string) {
  return useQuery({
    queryKey: ['renames', fileId],
    queryFn: () => fetchJSON<RenameRecord[]>(`/api/files/${fileId}/renames`),
  })
}

export function useHistory() {
  return useQuery({
    queryKey: ['history'],
    queryFn: () => fetchJSON<HistoryEntry[]>('/api/history?limit=200'),
  })
}

export function useSSE(queryClient: QueryClient) {
  useEffect(() => {
    const es = new EventSource('/api/events')
    es.onmessage = () => {
      queryClient.invalidateQueries({ queryKey: ['history'] })
      queryClient.invalidateQueries({ queryKey: ['stats'] })
    }
    es.onerror = () => {
      // EventSource auto-reconnects; log for debugging
      console.warn('SSE connection error, will retry automatically')
    }
    return () => es.close()
  }, [queryClient])
}

export function downloadSnapshotUrl(id: string): string {
  return `/api/snapshots/${id}/download`
}

export function databaseDownloadUrl(): string {
  return '/api/database/download'
}

export function useStripWatchDir(): (filePath: string) => string {
  const { data: stats } = useStats()
  return useCallback(
    (filePath: string): string => {
      if (stats?.watchDirs.length === 1) {
        const prefix = stats.watchDirs[0]
        if (filePath.startsWith(prefix + '/')) {
          return filePath.slice(prefix.length + 1)
        }
      }
      return filePath
    },
    [stats],
  )
}
