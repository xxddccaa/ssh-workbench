import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Application } from './app/application'
import { Notices } from './shared/ui/notices'
import '@xterm/xterm/css/xterm.css'
import './shared/styles/app.css'

const queries = new QueryClient({
  defaultOptions: {
    queries: { retry: false, refetchOnWindowFocus: false, staleTime: 10000 },
    mutations: { retry: false },
  },
})
const root = document.getElementById('root')
if (!root) throw new Error('Application root is missing')

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queries}>
      <Notices>
        <Application />
      </Notices>
    </QueryClientProvider>
  </StrictMode>,
)
