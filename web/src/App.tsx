import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useSSE, useStats } from './lib/api'
import { useRoute } from './lib/router'
import { useWatchSetState } from './lib/watchSetState'
import Layout from './components/Layout'
import Dashboard from './components/Dashboard'
import FilePage from './components/FilePage'

export default function App() {
  const queryClient = useQueryClient()
  useSSE(queryClient)

  const route = useRoute()
  const { data: stats } = useStats()
  const { activeWatchSet, setActiveWatchSet } = useWatchSetState()

  // When there are 2+ watch sets and no active tab yet, activate the first one
  useEffect(() => {
    if (stats?.watchSets && stats.watchSets.length >= 2 && activeWatchSet === null) {
      setActiveWatchSet(stats.watchSets[0].name)
    }
  }, [stats, activeWatchSet, setActiveWatchSet])

  return (
    <Layout>
      {route.page === 'dashboard' && <Dashboard query={route.query} />}
      {route.page === 'file' && (
        <FilePage
          fileId={route.fileId}
          fromId={route.fromId}
          toId={route.toId}
        />
      )}
    </Layout>
  )
}
