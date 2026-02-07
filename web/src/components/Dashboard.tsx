import { useState, useMemo, useEffect, useRef } from 'react'
import { useHistory, useStripWatchDir } from '../lib/api'
import { formatDate, formatBytes } from '../lib/format'
import { navigate, replaceUrl } from '../lib/router'

const PAGE_SIZE = 50


interface DashboardProps {
  query: string
}

export default function Dashboard({ query: initialQuery }: DashboardProps) {
  const [page, setPage] = useState(0)
  const [query, setQuery] = useState(initialQuery)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  const offset = page * PAGE_SIZE
  const { data, isLoading, error } = useHistory(PAGE_SIZE, offset)
  const stripWatchDir = useStripWatchDir()

  useEffect(() => {
    setQuery(initialQuery)
  }, [initialQuery])

  useEffect(() => {
    setPage(0)
  }, [initialQuery])

  function handleQueryChange(value: string) {
    setQuery(value)
    setPage(0)
    clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      replaceUrl(value ? `/?q=${encodeURIComponent(value)}` : '/')
    }, 300)
  }

  useEffect(() => {
    return () => clearTimeout(debounceRef.current)
  }, [])

  const filtered = useMemo(() => {
    if (!data) return []
    if (!query) return data.entries
    const lower = query.toLowerCase()
    return data.entries.filter((e) => {
      if (e.filePath.toLowerCase().includes(lower)) return true
      if (e.oldFilePath && e.oldFilePath.toLowerCase().includes(lower)) return true
      return false
    })
  }, [data, query])

  const hasMore = data?.hasMore ?? false

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
        <table className="w-full border border-gray-200 rounded-md overflow-hidden text-sm">
          <thead>
            <tr className="bg-gray-50 text-left text-gray-600">
              <th className="px-3 py-2 font-medium">Date</th>
              <th className="px-3 py-2 font-medium">File</th>
              <th className="px-3 py-2 font-medium text-right">Size(Byte)</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-200">
            {filtered.map((entry) => (
                <tr
                  key={`${entry.entryType}-${entry.snapshotId}`}
                  className="cursor-pointer hover:bg-blue-100"
                  onClick={() =>
                    entry.entryType === 'rename'
                      ? navigate(`/files/${entry.fileId}`)
                      : navigate(`/files/${entry.fileId}/diff/${entry.snapshotId}`)
                  }
                >
                  <td className="px-3 py-2 text-gray-500 whitespace-nowrap">
                    {formatDate(entry.timestamp)}
                  </td>
                  <td className="px-3 py-2 font-mono truncate">
                    {entry.entryType === 'rename' ? (
                      <span>
                        <span className="text-gray-400">{stripWatchDir(entry.oldFilePath ?? '')}</span>
                        <span className="text-gray-400 mx-1">&rarr;</span>
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
                      </span>
                    ) : (
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
                    )}
                  </td>
                  <td className="px-3 py-2 text-gray-500 text-right whitespace-nowrap">
                    {entry.entryType === 'rename' ? (
                      <span className="text-xs font-medium text-amber-600 bg-amber-50 px-1.5 py-0.5 rounded">rename</span>
                    ) : (
                      formatBytes(entry.size)
                    )}
                  </td>
                </tr>
            ))}
          </tbody>
        </table>
      )}
      {(page > 0 || hasMore) && (
        <div className="flex items-center justify-between">
          <div>
            {page > 0 && (
              <button
                type="button"
                onClick={() => setPage(page - 1)}
                className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
              >
                Previous
              </button>
            )}
          </div>
          <div>
            {hasMore && (
              <button
                type="button"
                onClick={() => setPage(page + 1)}
                className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
              >
                Next
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
