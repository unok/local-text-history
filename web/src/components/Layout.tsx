import { type ReactNode } from 'react'
import { useStats, databaseDownloadUrl } from '../lib/api'
import { formatBytes } from '../lib/format'
import { navigate } from '../lib/router'

export default function Layout({ children }: { children: ReactNode }) {
  const { data: stats, error } = useStats()

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white border-b border-gray-200 shadow-sm">
        <div className="max-w-7xl mx-auto px-4 py-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <a
              href="/"
              className="text-xl font-bold text-gray-800 hover:text-blue-600"
              onClick={(e) => {
                e.preventDefault()
                navigate('/')
              }}
            >
              File History Tracker
            </a>
            {stats?.watchDirs.length === 1 && (
              <span className="text-sm text-gray-400 font-mono">
                {stats.watchDirs[0]}
              </span>
            )}
          </div>
          {error && (
            <span className="text-red-500 text-sm">Failed to load stats</span>
          )}
          {stats && (
            <div className="flex items-center gap-4 text-sm text-gray-500">
              <span>{stats.totalFiles} files</span>
              <span>{stats.totalSnapshots} snapshots</span>
              <span>{formatBytes(stats.totalSize)}</span>
              <a
                href={databaseDownloadUrl()}
                className="px-3 py-1 text-xs font-medium text-blue-600 bg-blue-50 border border-blue-200 rounded hover:bg-blue-100 transition-colors"
                title="Download database snapshot"
              >
                DB Download
              </a>
            </div>
          )}
        </div>
      </header>
      <main className="max-w-7xl mx-auto px-4 py-6">{children}</main>
    </div>
  )
}
