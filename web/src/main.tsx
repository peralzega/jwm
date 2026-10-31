import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter, RouterProvider } from 'react-router'

import { TooltipProvider } from './components/ui/tooltip'
import './index.css'
import { applyDensity } from './density'
import { routes } from './routes'

applyDensity()

const queryClient = new QueryClient({
  defaultOptions: { queries: { refetchOnWindowFocus: false } },
})
const router = createBrowserRouter(routes)

const root = document.getElementById('root')
if (!root) throw new Error('elemen #root tidak ditemukan')

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>,
)
