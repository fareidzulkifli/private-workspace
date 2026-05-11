import { lazy, Suspense, useEffect, useState } from 'react'
import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom'
import Sidebar from './components/Sidebar'
import { refreshSession } from './lib/api'

const Login = lazy(() => import('./routes/Login'))
const Dashboard = lazy(() => import('./routes/Dashboard'))
const OrgBoard = lazy(() => import('./routes/OrgBoard'))
const ProjectSettings = lazy(() => import('./routes/ProjectSettings'))
const PromptVault = lazy(() => import('./components/prompts/PromptVault'))
const GitNoteRoute = lazy(() => import('./routes/GitNoteRoute'))
const ShareRoute = lazy(() => import('./routes/ShareRoute'))
const Wallet = lazy(() => import('./routes/Wallet'))

const CHUNK_RELOAD_KEY = 'private-workspace:chunk-reload-at'

window.addEventListener('vite:preloadError', event => {
  event.preventDefault()
  const now = Date.now()
  const lastReload = Number(window.sessionStorage.getItem(CHUNK_RELOAD_KEY) || 0)
  if (now - lastReload > 30000) {
    window.sessionStorage.setItem(CHUNK_RELOAD_KEY, String(now))
    window.location.reload()
  }
})

function RouteFallback() {
  return <div className="app-route-loading">Loading...</div>
}

function Shell() {
  const location = useLocation()
  const [loading, setLoading] = useState(true)
  const [authenticated, setAuthenticated] = useState(false)
  const isPublicShare = location.pathname === '/share' || location.pathname.startsWith('/share/')
  const isLogin = location.pathname === '/login'

  useEffect(() => {
    let mounted = true
    const load = () => refreshSession()
      .then(session => {
        if (mounted) setAuthenticated(!!session.authenticated)
      })
      .catch(() => {
        if (mounted) setAuthenticated(false)
      })
      .finally(() => {
        if (mounted) setLoading(false)
      })
    load()
    window.addEventListener('authChanged', load)
    return () => {
      mounted = false
      window.removeEventListener('authChanged', load)
    }
  }, [])

  if (loading) {
    return <div className="app-container"><main className="main-content" /></div>
  }

  if (!authenticated && !isLogin && !isPublicShare) {
    return <Navigate to="/login" replace />
  }
  if (authenticated && isLogin) {
    return <Navigate to="/dashboard" replace />
  }

  return (
    <div className="app-container">
      <Sidebar />
      <main className="main-content">
        <Suspense fallback={<RouteFallback />}>
          <Outlet />
        </Suspense>
      </main>
    </div>
  )
}

export default function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route path="/login" element={<Login />} />
        <Route path="/dashboard" element={<Dashboard />} />
        <Route path="/task/dashboard" element={<Navigate to="/dashboard" replace />} />
        <Route path="/task/org/:slug" element={<OrgBoard />} />
        <Route path="/projects/:id/settings" element={<ProjectSettings />} />
        <Route path="/prompts" element={<PromptVault />} />
        <Route path="/gitnote/*" element={<GitNoteRoute />} />
        <Route path="/wallet" element={<Wallet />} />
        <Route path="/wallet/review" element={<Wallet />} />
        <Route path="/wallet/reports" element={<Wallet />} />
        <Route path="/wallet/settings" element={<Wallet />} />
        <Route path="/share/:token/*" element={<ShareRoute />} />
        <Route path="/share/:token" element={<ShareRoute />} />
        <Route path="*" element={<Navigate to="/dashboard" replace />} />
      </Route>
    </Routes>
  )
}
