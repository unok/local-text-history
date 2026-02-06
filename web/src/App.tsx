import { useQueryClient } from '@tanstack/react-query'
import { useSSE } from './lib/api'
import { useRoute } from './lib/router'
import Layout from './components/Layout'
import Dashboard from './components/Dashboard'
import FilePage from './components/FilePage'

export default function App() {
  const queryClient = useQueryClient()
  useSSE(queryClient)

  const route = useRoute()

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
