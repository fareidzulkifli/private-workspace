'use client';

import { useState, useEffect } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import LogoutButton from './LogoutButton'
import {
  LayoutDashboard,
  Plus,
  ChevronRight,
  Briefcase,
  Settings,
  LogOut,
  MoreVertical,
  Layers,
  CheckCircle2,
  Sun,
  Moon,
  Monitor,
  BookOpen,
  MessageSquareText,
  Wallet
} from 'lucide-react'

export default function Sidebar() {
  const [navData, setNavData] = useState([])
  const [loading, setLoading] = useState(true)
  const [theme, setTheme] = useState('dark')
  const [isMobileOpen, setIsMobileOpen] = useState(false)
  const [isCollapsed, setIsCollapsed] = useState(false)
  const params = useParams()
  const { pathname } = useLocation()
  const currentOrgSlug = params?.slug

  const fetchNav = async () => {
    try {
      const res = await fetch('/api/nav')
      const data = await res.json()
      if (Array.isArray(data)) {
        setNavData(data)
      }
    } catch (err) {
      console.error('Failed to fetch nav data', err)
    } finally {
      setLoading(false)
    }
  }

  // Initialize theme from localStorage or system preference
  useEffect(() => {
    const savedTheme = localStorage.getItem('theme') || 'dark'
    setTheme(savedTheme)
    document.documentElement.setAttribute('data-theme', savedTheme)
    setIsCollapsed(localStorage.getItem('sidebar-collapsed') === 'true')
  }, [])

  const toggleTheme = () => {
    const nextTheme = theme === 'dark' ? 'light' : 'dark'
    setTheme(nextTheme)
    localStorage.setItem('theme', nextTheme)
    document.documentElement.setAttribute('data-theme', nextTheme)
  }

  useEffect(() => {
    const handleToggle = () => setIsMobileOpen(prev => !prev)
    window.addEventListener('toggle-sidebar', handleToggle)
    return () => window.removeEventListener('toggle-sidebar', handleToggle)
  }, [])

  useEffect(() => {
    // Close sidebar on route change (mobile)
    setIsMobileOpen(false)
  }, [pathname, currentOrgSlug])

  useEffect(() => {
    if (pathname === '/login' || pathname.startsWith('/share')) return
    fetchNav()
  }, [])

  useEffect(() => {
    window.addEventListener('taskUpdated', fetchNav)
    const interval = setInterval(fetchNav, 60000)
    return () => {
      window.removeEventListener('taskUpdated', fetchNav)
      clearInterval(interval)
    }
  }, [])

  const handleCreateOrg = async () => {
    const name = prompt('Organization Name:')
    if (!name) return

    try {
      const res = await fetch('/api/orgs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name,
          order_index: navData.length
        })
      })
      const newOrg = await res.json()
      if (newOrg.error) throw new Error(newOrg.error)
      fetchNav()
    } catch (err) {
      alert('Error: ' + err.message)
    }
  }

  if (pathname === '/login' || pathname.startsWith('/share')) return null

  return (
    <>
      {/* Mobile Overlay Backdrop */}
      <div
        className={`sidebar-backdrop ${isMobileOpen ? 'open' : ''}`}
        onClick={() => setIsMobileOpen(false)}
      />

      <aside
        className={`sidebar ${isMobileOpen ? 'mobile-open' : ''} ${isCollapsed ? 'collapsed' : ''}`}
        style={{ background: 'var(--surface)' }}
      >
        <div className="sidebar-header">
          <div className="sidebar-header-main" style={{ justifyContent: isCollapsed ? 'center' : 'space-between' }}>
            <Link to="/dashboard" className="sidebar-logo">
              <div className="sidebar-logo-icon">
                <Layers size={14} color="#fff" strokeWidth={3} />
              </div>
              <span style={{ color: 'var(--text)' }}>Private Workspace</span>
            </Link>
            <div className="sidebar-header-actions">
              <button
                onClick={toggleTheme}
                className="btn-ghost sidebar-theme-btn"
                title={theme === 'dark' ? 'Switch to Technical Light' : 'Switch to Industrial Dark'}
                aria-label={theme === 'dark' ? 'Switch to Technical Light' : 'Switch to Industrial Dark'}
              >
                {theme === 'dark' ? <Sun size={14} /> : <Moon size={14} />}
              </button>

              <button
                className="btn-ghost mobile-close-btn"
                onClick={() => {
                  if (window.innerWidth <= 768) {
                    setIsMobileOpen(false)
                  } else {
                    setIsCollapsed(prev => {
                      const next = !prev
                      localStorage.setItem('sidebar-collapsed', String(next))
                      return next
                    })
                  }
                }}
                style={{ padding: '4px' }}
                title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              >
                <ChevronRight size={18} style={{ transform: (isMobileOpen || !isCollapsed) ? 'rotate(180deg)' : 'rotate(0deg)', transition: 'transform 0.25s' }} />
              </button>
            </div>
          </div>
        </div>

      <nav className="sidebar-nav">
        <div className="nav-section">
          <Link to="/dashboard" className={`sidebar-link ${pathname === '/dashboard' ? 'active' : ''}`} style={{ fontSize: '12px', fontWeight: '700', fontFamily: 'monospace' }}>
            <LayoutDashboard size={16} />
            <span>Dashboard</span>
          </Link>
          <Link to="/gitnote" className={`sidebar-link ${pathname.startsWith('/gitnote') ? 'active' : ''}`} style={{ fontSize: '12px', fontWeight: '700', fontFamily: 'monospace', marginTop: '2px' }}>
            <BookOpen size={16} />
            <span>GitNotes</span>
          </Link>
          <Link to="/prompts" className={`sidebar-link ${pathname.startsWith('/prompts') ? 'active' : ''}`} style={{ fontSize: '12px', fontWeight: '700', fontFamily: 'monospace', marginTop: '2px' }}>
            <MessageSquareText size={16} />
            <span>Prompts</span>
          </Link>
          <Link to="/wallet" className={`sidebar-link ${pathname.startsWith('/wallet') ? 'active' : ''}`} style={{ fontSize: '12px', fontWeight: '700', fontFamily: 'monospace', marginTop: '2px' }}>
            <Wallet size={16} />
            <span>Wallet</span>
          </Link>
        </div>

        <div className="nav-section" style={{ marginTop: '24px' }}>
          <div className="sidebar-section-header">
            <h3 className="nav-section-title" style={{ fontSize: '10px', fontWeight: '800', letterSpacing: '0.15em' }}>Task Manager</h3>
            <button
              onClick={handleCreateOrg}
              className="btn-ghost sidebar-section-action"
              title="Create organization"
              aria-label="Create organization"
            >
              <Plus size={13} />
              <span>Create ORG</span>
            </button>
          </div>
          {loading ? (
            <div style={{ padding: '12px', color: 'var(--text-disabled)', fontSize: '10px', fontFamily: 'monospace' }}>FETCHING_NODES...</div>
          ) : (
            <div className="sidebar-orgs">
              {navData.map(org => (
                <div key={org.id} className="sidebar-org-group">
                  <Link
                    to={`/task/org/${org.slug}`}
                    className={`sidebar-link ${currentOrgSlug === org.slug ? 'active' : ''}`}
                    style={{ marginTop: '2px', fontSize: '12px', fontWeight: '700', fontFamily: 'monospace' }}
                  >
                    <Briefcase size={16} />
                    <span style={{ flexGrow: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{org.name.toUpperCase()}</span>
                    {currentOrgSlug === org.slug && <ChevronRight size={12} opacity={0.5} />}
                  </Link>

                  {currentOrgSlug === org.slug && (
                    <div className="sidebar-projects" style={{ borderLeft: '1px solid var(--border-strong)', marginLeft: '20px', paddingLeft: '0' }}>
                      {org.projects.map(project => (
                        <div key={project.id} className="sidebar-project-item" style={{ paddingLeft: '16px' }}>
                          <span className="sidebar-project-name" title={project.name} style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '11px', fontFamily: 'monospace' }}>
                            <div style={{ width: '4px', height: '1px', background: 'var(--text-disabled)' }}></div>
                            {project.name.toUpperCase()}
                          </span>
                          {project.incomplete_tasks_count > 0 && (
                            <span style={{
                              background: 'var(--accent-subtle)',
                              color: 'var(--accent)',
                              fontSize: '9px',
                              fontWeight: '900',
                              padding: '1px 5px',
                              borderRadius: '4px',
                              border: '1px solid var(--accent-muted)'
                            }}>
                              {project.incomplete_tasks_count.toString().padStart(2, '0')}
                            </span>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </nav>

      <div style={{ padding: '16px', borderTop: '1px solid var(--border-strong)', marginTop: 'auto', background: 'var(--background)' }}>
        <LogoutButton style={{ width: '100%', background: 'transparent', color: 'var(--text-muted)', border: '1px solid var(--border-strong)', fontSize: '11px', fontWeight: '800', fontFamily: 'monospace' }} />
      </div>
    </aside>
    </>
  )
}
