import { useState, useMemo, useEffect, useRef } from 'react'
import { useHistory, useStripWatchDir } from '../lib/api'
import { formatDate, formatBytes } from '../lib/format'
import { navigate, replaceUrl } from '../lib/router'

interface DashboardProps {
  query: string
}

export default function Dashboard({ query: initialQuery }: DashboardProps) {
  const { data: entries, isLoading, error } = useHistory()
  const stripWatchDir = useStripWatchDir()
  const [query, setQuery] = useState(initialQuery)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  useEffect(() => {
    setQuery(initialQuery)
  }, [initialQuery])

  function handleQueryChange(value: string) {
    setQuery(value)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      replaceUrl(value ? `/?q=${encodeURIComponent(value)}` : '/')
    }, 300)
  }

  useEffect(() => {
    return () => clearTimeout(debounceRef.current)
  }, [])

  const filtered = useMemo(() => {
    if (!entries) return []
    if (!query) return entries
    const lower = query.toLowerCase()
    return entries.filter((e) => e.filePath.toLowerCase().includes(lower))
  }, [entries, query])

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-gray-800">Recent Changes</h2>
      <input
        type="text"
        value={query}
        onChange={(e) => handleQueryChange(e.target.value)}
        placeholder="Search by file path..."
        aria-label="Search recent changes"
        className="w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
      />
      {isLoading && <p className="text-gray-500 text-sm">Loading...</p>}
      {error && (
        <p className="text-red-500 text-sm">Error: {error.message}</p>
      )}
      {!isLoading && filtered.length === 0 && (
        <p className="text-gray-500 text-sm">No recent changes found.</p>
      )}
      {filtered.length > 0 && (
        <ul className="divide-y divide-gray-200 border border-gray-200 rounded-md overflow-hidden">
          {filtered.map((entry) => (
            <li
              key={entry.snapshotId}
              className="px-3 py-2 cursor-pointer hover:bg-blue-50 bg-white"
              onClick={() =>
                navigate(
                  `/files/${entry.fileId}/diff/${entry.snapshotId}`,
                )
              }
            >
              <p className="text-sm font-mono truncate">
                <a
                  href={`/files/${entry.fileId}`}
                  className="text-blue-600 hover:underline"
                  onClick={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    navigate(`/files/${entry.fileId}`)
                  }}
                >
                  {stripWatchDir(entry.filePath)}
                </a>
              </p>
              <p className="text-xs text-gray-500">
                {formatDate(entry.timestamp)} &middot;{' '}
                {formatBytes(entry.size)} &middot;{' '}
                {entry.hash.substring(0, 8)}
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
