import { useMemo } from 'react'
import {
  useFile,
  useSnapshots,
  useRenames,
  useStripWatchDir,
  downloadSnapshotUrl,
  type Snapshot,
  type RenameRecord,
} from '../lib/api'
import { formatDate, formatBytes } from '../lib/format'
import { navigate, replaceUrl } from '../lib/router'
import DiffView from './DiffView'

interface FilePageProps {
  fileId: string
  fromId: string | null // explicit from (null = auto-resolve)
  toId: string | null // selected to (null = none selected)
}

function RenameHistory({
  renames,
  stripWatchDir,
}: {
  renames: RenameRecord[]
  stripWatchDir: (path: string) => string
}) {
  return (
    <div className="mt-2">
      <p className="text-xs font-semibold text-gray-500 mb-1">
        Rename History
      </p>
      <ul className="text-xs text-gray-600 space-y-0.5">
        {renames.map((r) => {
          return (
            <li key={r.id} className="flex items-center gap-1">
              <a
                href={`/files/${r.oldFileId}`}
                className="text-blue-600 hover:underline font-mono"
                onClick={(e) => {
                  e.preventDefault()
                  navigate(`/files/${r.oldFileId}`)
                }}
              >
                {stripWatchDir(r.oldPath)}
              </a>
              <span className="text-gray-400">&rarr;</span>
              <a
                href={`/files/${r.newFileId}`}
                className="text-blue-600 hover:underline font-mono"
                onClick={(e) => {
                  e.preventDefault()
                  navigate(`/files/${r.newFileId}`)
                }}
              >
                {stripWatchDir(r.newPath)}
              </a>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function resolvePreviousSnapshot(
  snapshotId: string,
  snapshots: Snapshot[],
): string | null {
  const idx = snapshots.findIndex((s) => s.id === snapshotId)
  if (idx < 0) return null
  // snapshots are newest-first, so "previous" is idx+1
  return idx + 1 < snapshots.length ? snapshots[idx + 1].id : null
}

export default function FilePage({ fileId, fromId, toId }: FilePageProps) {
  const {
    data: file,
    isLoading: fileLoading,
    error: fileError,
  } = useFile(fileId)
  const {
    data: snapshots,
    isLoading: snapsLoading,
    error: snapsError,
  } = useSnapshots(fileId)
  const { data: renames } = useRenames(fileId)
  const stripWatchDir = useStripWatchDir()

  // Resolve actual from/to for the DiffView
  const [resolvedFromId, resolvedToId] = useMemo(():
    | [string | null, string | null] => {
    if (!toId || !snapshots) return [null, null]
    if (fromId) return [fromId, toId]
    // Auto-resolve: find previous snapshot
    return [resolvePreviousSnapshot(toId, snapshots), toId]
  }, [fromId, toId, snapshots])

  if (fileLoading || snapsLoading) {
    return <p className="text-gray-500 text-sm">Loading...</p>
  }
  if (fileError) {
    return (
      <p className="text-red-500 text-sm">Error: {fileError.message}</p>
    )
  }

  function handleRowClick(snapId: string) {
    // Default: show diff with previous (auto-resolve from)
    replaceUrl(`/files/${fileId}/diff/${snapId}`)
  }

  function handleSetFrom(e: React.MouseEvent, snapId: string) {
    e.stopPropagation()
    if (toId) {
      replaceUrl(`/files/${fileId}/diff/${snapId}/${toId}`)
    }
  }

  function handleSetTo(e: React.MouseEvent, snapId: string) {
    e.stopPropagation()
    if (resolvedFromId) {
      replaceUrl(`/files/${fileId}/diff/${resolvedFromId}/${snapId}`)
    } else {
      replaceUrl(`/files/${fileId}/diff/${snapId}`)
    }
  }

  const hasDiff = resolvedToId !== null

  return (
    <div className="space-y-4">
      <div>
        <a
          href="/"
          className="text-blue-600 hover:underline text-sm"
          onClick={(e) => {
            e.preventDefault()
            navigate('/')
          }}
        >
          &larr; Back
        </a>
        <h2 className="text-lg font-mono font-semibold text-gray-800 mt-1">
          {file ? stripWatchDir(file.path) : ''}
        </h2>
        {renames && renames.length > 0 && (
          <RenameHistory
            renames={renames}
            stripWatchDir={stripWatchDir}
          />
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-1">
          <h3 className="text-sm font-semibold text-gray-700 mb-2">
            Snapshots
          </h3>
          {!hasDiff && (
            <p className="text-xs text-gray-500 mb-2">
              Click a snapshot to compare with its previous version.
            </p>
          )}
          {hasDiff && (
            <p className="text-xs text-gray-500 mb-2">
              Use From / To buttons to change comparison range.
            </p>
          )}
          {snapsError && (
            <p className="text-red-500 text-sm">
              Error: {snapsError.message}
            </p>
          )}
          {snapshots && snapshots.length === 0 && (
            <p className="text-gray-500 text-sm">No snapshots.</p>
          )}
          {snapshots && snapshots.length > 0 && (
            <ul className="divide-y divide-gray-200 border border-gray-200 rounded-md overflow-hidden">
              {snapshots.map((snap) => {
                const isFrom = snap.id === resolvedFromId
                const isTo = snap.id === resolvedToId
                return (
                  <li
                    key={snap.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => handleRowClick(snap.id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        handleRowClick(snap.id)
                      }
                    }}
                    className={`px-3 py-2 cursor-pointer hover:bg-blue-50 ${
                      isFrom || isTo ? 'bg-blue-100' : 'bg-white'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <div className="min-w-0">
                        <p className="text-sm text-gray-800">
                          {formatDate(snap.timestamp)}
                        </p>
                        <p className="text-xs text-gray-500">
                          {formatBytes(snap.size)} &middot;{' '}
                          {snap.hash.substring(0, 8)}
                        </p>
                      </div>
                      <div className="flex items-center gap-1 shrink-0 ml-2">
                        {isFrom && (
                          <span className="text-xs bg-orange-500 text-white px-2 py-0.5 rounded">
                            From
                          </span>
                        )}
                        {isTo && (
                          <span className="text-xs bg-blue-500 text-white px-2 py-0.5 rounded">
                            To
                          </span>
                        )}
                        {hasDiff && !isFrom && !isTo && (
                          <>
                            <button
                              onClick={(e) => handleSetFrom(e, snap.id)}
                              className="text-xs text-orange-600 hover:bg-orange-100 px-1.5 py-0.5 rounded"
                            >
                              From
                            </button>
                            <button
                              onClick={(e) => handleSetTo(e, snap.id)}
                              className="text-xs text-blue-600 hover:bg-blue-100 px-1.5 py-0.5 rounded"
                            >
                              To
                            </button>
                          </>
                        )}
                        <a
                          href={downloadSnapshotUrl(snap.id)}
                          onClick={(e) => e.stopPropagation()}
                          className="text-blue-500 hover:text-blue-700 text-xs"
                          title="Download"
                        >
                          DL
                        </a>
                      </div>
                    </div>
                  </li>
                )
              })}
            </ul>
          )}
        </div>

        <div className="lg:col-span-2 min-w-0">
          {hasDiff ? (
            <DiffView fromId={resolvedFromId} toId={resolvedToId} />
          ) : (
            <p className="text-gray-500 text-sm">
              Select a snapshot to view diff.
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
