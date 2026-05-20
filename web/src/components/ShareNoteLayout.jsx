'use client'

import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { LogIn } from 'lucide-react'
import GitNoteViewer from './GitNoteViewer'

export default function ShareNoteLayout({ token, filePath }) {
  const [theme, setTheme] = useState('light')
  const [selectedPath, setSelectedPath] = useState(filePath)
  const rawBase = `/api/share/gitnote/${encodeURIComponent(token)}/raw`
  const downloadBase = `/api/share/gitnote/${encodeURIComponent(token)}/download`

  useEffect(() => {
    setSelectedPath(filePath)
  }, [filePath])

  useEffect(() => {
    if (filePath || !token) return

    let active = true
    fetch(`/api/share/gitnote/${encodeURIComponent(token)}`)
      .then(r => r.json())
      .then(data => {
        if (active && data.pathPrefix) setSelectedPath(data.pathPrefix)
      })
      .catch(() => {})

    return () => {
      active = false
    }
  }, [filePath, token])

  useEffect(() => {
    const onPop = () => {
      const prefix = `/share/${token}/`
      setSelectedPath(window.location.pathname.startsWith(prefix)
        ? decodeURIComponent(window.location.pathname.slice(prefix.length))
        : null)
    }
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [token])

  return (
    <div className="gitnote-page" data-theme={theme}>
      <header className="gitnote-topbar">
        <div className="gitnote-brand">
          <span className="gitnote-brand-name">BA Notes</span>
          <span className="gitnote-brand-sep">·</span>
          <span className="gitnote-brand-sub">fareidzulkifli</span>
        </div>
        <Link to="/login" className="share-signin-btn">
          <LogIn size={12} />
          <span>Log In</span>
        </Link>
      </header>
      <div className="share-body">
        <GitNoteViewer
          filePath={selectedPath}
          theme={theme}
          rawBase={rawBase}
          downloadBase={downloadBase}
          disableShare
          showBreadcrumbs={false}
          onToggleTheme={() => setTheme(t => t === 'dark' ? 'light' : 'dark')}
        />
      </div>
    </div>
  )
}
