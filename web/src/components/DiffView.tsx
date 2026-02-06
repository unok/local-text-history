import { useState } from 'react'
import { useDiff } from '../lib/api'
import { html as diff2htmlHtml } from 'diff2html'
import '../styles/diff2html-scoped.css'

type OutputFormat = 'side-by-side' | 'line-by-line'

interface DiffViewProps {
  fromId: string | null
  toId: string | null
}

export default function DiffView({ fromId, toId }: DiffViewProps) {
  const [format, setFormat] = useState<OutputFormat>('side-by-side')
  const { data, isLoading, error } = useDiff(fromId, toId)

  if (toId === null) {
    return (
      <div className="text-gray-500 text-sm">
        Select a snapshot to view diff.
      </div>
    )
  }

  if (isLoading) {
    return <p className="text-gray-500 text-sm">Loading diff...</p>
  }

  if (error) {
    return (
      <p className="text-red-500 text-sm">
        Error loading diff: {error.message}
      </p>
    )
  }

  if (!data || data.diff === '') {
    return <p className="text-gray-500 text-sm">No differences found.</p>
  }

  const diffHtml = diff2htmlHtml(data.diff, {
    drawFileList: false,
    matching: 'lines',
    outputFormat: format,
  })

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-gray-700">
          {data.from
            ? <>Diff: Snapshot {data.from.substring(0, 8)} &rarr; Snapshot {data.to.substring(0, 8)}</>
            : <>Initial Snapshot {data.to.substring(0, 8)}</>
          }
        </h3>
        <div className="flex gap-2">
          <button
            onClick={() => setFormat('side-by-side')}
            className={`px-3 py-1 text-sm rounded ${
              format === 'side-by-side'
                ? 'bg-blue-500 text-white'
                : 'bg-gray-200 text-gray-700 hover:bg-gray-300'
            }`}
          >
            Side by Side
          </button>
          <button
            onClick={() => setFormat('line-by-line')}
            className={`px-3 py-1 text-sm rounded ${
              format === 'line-by-line'
                ? 'bg-blue-500 text-white'
                : 'bg-gray-200 text-gray-700 hover:bg-gray-300'
            }`}
          >
            Inline
          </button>
        </div>
      </div>
      <div
        className="d2h-scope border border-gray-200 rounded-md overflow-auto"
        dangerouslySetInnerHTML={{ __html: diffHtml }}
      />
    </div>
  )
}
