import { type ReactNode } from 'react'
import { useStats, databaseDownloadUrl } from '../lib/api'
import { formatBytes } from '../lib/format'
import { navigate } from '../lib/router'
import { useTheme } from '../lib/theme'

function SunIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="12" cy="12" r="5" />
      <line x1="12" y1="1" x2="12" y2="3" />
      <line x1="12" y1="21" x2="12" y2="23" />
      <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
      <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
      <line x1="1" y1="12" x2="3" y2="12" />
      <line x1="21" y1="12" x2="23" y2="12" />
      <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
      <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
    </svg>
  )
}

function MoonIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
    </svg>
  )
}

export default function Layout({ children }: { children: ReactNode }) {
  const { data: stats, error } = useStats()
  const { theme, toggleTheme } = useTheme()

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900">
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 shadow-sm">
        <div className="max-w-7xl mx-auto px-4 py-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <a
              href="/"
              className="text-xl font-bold text-gray-800 dark:text-gray-100 hover:text-blue-600 dark:hover:text-blue-400"
              onClick={(e) => {
                e.preventDefault()
                navigate('/')
              }}
            >
              File History Tracker
            </a>
            {stats?.watchDirs.length === 1 && (
              <span className="text-sm text-gray-400 dark:text-gray-500 font-mono">
                {stats.watchDirs[0]}
              </span>
            )}
          </div>
          {error && (
            <span className="text-red-500 dark:text-red-400 text-sm">Failed to load stats</span>
          )}
          {stats && (
            <div className="flex items-center gap-4 text-sm text-gray-500 dark:text-gray-400">
              <span>{stats.totalFiles} files</span>
              <span>{stats.totalSnapshots} snapshots</span>
              <span>{formatBytes(stats.totalSize)}</span>
              <a
                href={databaseDownloadUrl()}
                className="px-3 py-1 text-xs font-medium text-blue-600 dark:text-blue-400 bg-blue-50 dark:bg-blue-900/30 border border-blue-200 dark:border-blue-700 rounded hover:bg-blue-100 dark:hover:bg-blue-800 transition-colors"
                title="Download database snapshot"
              >
                DB Download
              </a>
              <button
                type="button"
                onClick={toggleTheme}
                className="p-1.5 rounded text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
                title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
              >
                {theme === 'dark' ? <SunIcon /> : <MoonIcon />}
              </button>
            </div>
          )}
        </div>
      </header>
      <main className="max-w-7xl mx-auto px-4 py-6">{children}</main>
    </div>
  )
}
