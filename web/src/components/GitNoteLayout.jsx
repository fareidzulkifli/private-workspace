'use client'

import { useState, useEffect, useRef } from 'react'
import { LayoutDashboard, Menu, X, Sun, Moon } from 'lucide-react'
import GitNoteExplorer from './GitNoteExplorer'
import GitNoteViewer from './GitNoteViewer'

export default function GitNoteLayout({ initialPath }) {
  const [selectedPath, setSelectedPath] = useState(
    initialPath && initialPath.length > 0 ? initialPath.join('/') : null
  )
  const [explorerOpen, setExplorerOpen] = useState(false)
  const [desktopExplorerOpen, setDesktopExplorerOpen] = useState(true)
  const [isMobile, setIsMobile] = useState(false)

  const [theme, setTheme] = useState('light')
  const [explorerWidth, setExplorerWidth] = useState(300)
  const [isResizing, setIsResizing] = useState(false)

  useEffect(() => {
    const check = () => setIsMobile(window.innerWidth <= 768)
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])

  // Sync on initial load only — subsequent selections use pushState, not router
  useEffect(() => {
    const path = initialPath && initialPath.length > 0 ? initialPath.join('/') : null
    setSelectedPath(path)
  }, [])

  // Handle browser back/forward
  useEffect(() => {
    const handlePop = () => {
      const raw = window.location.pathname.replace(/^\/gitnote\/?/, '')
      setSelectedPath(raw || null)
    }
    window.addEventListener('popstate', handlePop)
    return () => window.removeEventListener('popstate', handlePop)
  }, [])

  useEffect(() => {
    if (!isResizing) return
    const handleMouseMove = (e) => {
      const newWidth = document.body.clientWidth - e.clientX
      if (newWidth > 150 && newWidth < 600) {
        setExplorerWidth(newWidth)
      }
    }
    const handleMouseUp = () => {
      setIsResizing(false)
      document.body.style.cursor = 'default'
    }
    document.addEventListener('mousemove', handleMouseMove)
    document.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.removeEventListener('mousemove', handleMouseMove)
      document.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isResizing])

  const handleSelect = (path) => {
    setSelectedPath(path)
    window.history.pushState(null, '', '/gitnote/' + path)
    if (isMobile) setExplorerOpen(false)
  }

  const handleMouseDown = (e) => {
    e.preventDefault()
    setIsResizing(true)
    document.body.style.cursor = 'ew-resize'
  }

  return (
    <div className="gitnote-page" data-theme={theme}>
      {/* Mobile-only bar: open app sidebar + toggle file explorer */}
      <div className="gitnote-mobile-bar">
        <button
          className="gitnote-icon-btn"
          onClick={() => window.dispatchEvent(new Event('toggle-sidebar'))}
          title="Open App Menu"
        >
          <LayoutDashboard size={18} />
        </button>
        <span className="gitnote-mobile-title">BA Notes</span>
        <button
          className="gitnote-icon-btn"
          onClick={() => setTheme(t => t === 'dark' ? 'light' : 'dark')}
          title="Toggle theme"
        >
          {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
        </button>
        <button
          className="gitnote-icon-btn"
          onClick={() => setExplorerOpen(o => !o)}
          title="Toggle Files"
        >
          {explorerOpen ? <X size={18} /> : <Menu size={18} />}
        </button>
      </div>

      <div className="gitnote-body">
        {explorerOpen && (
          <div className="sidebar-backdrop open gn-mobile-only" onClick={() => setExplorerOpen(false)} />
        )}

        <GitNoteViewer
          filePath={selectedPath}
          onToggleExplorer={() => setDesktopExplorerOpen(o => !o)}
          explorerVisible={desktopExplorerOpen}
          theme={theme}
          onToggleTheme={() => setTheme(t => t === 'dark' ? 'light' : 'dark')}
        />

        <aside
          className={`gitnote-explorer ${explorerOpen ? 'mobile-open' : 'mobile-hidden'} ${desktopExplorerOpen ? '' : 'desktop-hidden'}`}
          style={!isMobile ? { width: `${explorerWidth}px` } : undefined}
        >
          {!isMobile && (
            <div
              className="gitnote-explorer-resizer"
              onMouseDown={handleMouseDown}
            />
          )}
          <div className="gitnote-explorer-header">
            <span className="gitnote-explorer-label">Files</span>
          </div>
          <GitNoteExplorer selectedPath={selectedPath} onSelect={handleSelect} />
        </aside>
      </div>
    </div>
  )
}
