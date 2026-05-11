'use client'

import { useState, useEffect, useRef } from 'react'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/core'
import bash from 'highlight.js/lib/languages/bash'
import css from 'highlight.js/lib/languages/css'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import markdown from 'highlight.js/lib/languages/markdown'
import python from 'highlight.js/lib/languages/python'
import sql from 'highlight.js/lib/languages/sql'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'
import 'highlight.js/styles/atom-one-dark.css'
import mammothUrl from 'mammoth/mammoth.browser.min.js?url'
import { FileText, Loader2, AlertCircle, Download, ChevronRight, Share2, Check, PanelRight, Sun, Moon } from 'lucide-react'
import { apiFetch } from '../lib/api'

const TEXT_EXTENSIONS  = new Set(['md', 'txt', 'js', 'ts', 'jsx', 'tsx', 'py', 'json', 'yaml', 'yml', 'toml', 'sh', 'css', 'html', 'xml', 'csv', 'sql'])
const DOCX_EXTENSIONS  = new Set(['doc', 'docx'])
const EXCEL_EXTENSIONS = new Set(['xls', 'xlsx', 'xlxs'])
const IMAGE_EXTENSIONS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'ico'])

const highlightLanguages = {
  bash,
  css,
  javascript,
  json,
  markdown,
  python,
  sql,
  typescript,
  xml,
  yaml,
}

Object.entries(highlightLanguages).forEach(([name, language]) => {
  hljs.registerLanguage(name, language)
})

let mammothLoader = null

function loadMammoth() {
  if (typeof window === 'undefined') {
    return Promise.reject(new Error('DOCX preview is unavailable'))
  }
  if (window.mammoth) return Promise.resolve(window.mammoth)

  mammothLoader ||= new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = mammothUrl
    script.async = true
    script.onload = () => {
      if (window.mammoth) resolve(window.mammoth)
      else reject(new Error('DOCX parser failed to load'))
    }
    script.onerror = () => reject(new Error('DOCX parser failed to load'))
    document.head.appendChild(script)
  })

  return mammothLoader
}

function resolvePath(base, relative) {
  const parts = (base + '/' + relative).split('/')
  const resolved = []
  for (const part of parts) {
    if (part === '..') resolved.pop()
    else if (part !== '.') resolved.push(part)
  }
  return resolved.join('/')
}

function rewriteImageSrcs(html, filePath, rawBase) {
  const dir = filePath.split('/').slice(0, -1).join('/')
  return html.replace(/<img([^>]*?)\ssrc="([^"]*)"([^>]*?)>/gi, (match, before, src, after) => {
    if (/^https?:\/\/|^\/|^data:/.test(src)) return match
    const resolved = resolvePath(dir, src)
    return `<img${before} src="${rawBase}?path=${encodeURIComponent(resolved)}"${after}>`
  })
}

function Breadcrumb({ path }) {
  const parts = path.split('/')
  return (
    <div className="gn-breadcrumb">
      {parts.map((part, i) => (
        <span key={i} className="gn-breadcrumb-item">
          {i > 0 && <ChevronRight size={11} className="gn-breadcrumb-sep" />}
          <span className={i === parts.length - 1 ? 'gn-breadcrumb-current' : 'gn-breadcrumb-seg'}>
            {decodeURIComponent(part)}
          </span>
        </span>
      ))}
    </div>
  )
}

function stripFrontMatter(text) {
  const match = text.match(/^---\r?\n[\s\S]*?\n---\r?\n?/)
    || text.match(/^\+\+\+\r?\n[\s\S]*?\n\+\+\+\r?\n?/)
  return match ? text.slice(match[0].length) : text
}

marked.setOptions({ gfm: true, breaks: false })

function encodePathSegments(value) {
  return value.split('/').map(segment => {
    try {
      return encodeURIComponent(decodeURIComponent(segment))
    } catch {
      return encodeURIComponent(segment)
    }
  }).join('/')
}

function decodePathSegment(value) {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

async function copyToClipboard(text) {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch {
      if (typeof document === 'undefined') throw new Error('Clipboard unavailable')
    }
  }

  if (typeof document === 'undefined') throw new Error('Clipboard unavailable')

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  textarea.style.left = '-9999px'
  document.body.appendChild(textarea)
  textarea.select()

  try {
    if (!document.execCommand('copy')) {
      throw new Error('Copy command failed')
    }
  } finally {
    document.body.removeChild(textarea)
  }
}

export default function GitNoteViewer({ filePath, onToggleExplorer, explorerVisible, theme, onToggleTheme, rawBase = '/api/gitnote/raw', disableShare = false, showBreadcrumbs = true }) {
  const [content, setContent]           = useState(null)
  const [rendered, setRendered]         = useState(null) // { type: 'docx'|'excel', html?, sheets? }
  const [activeSheet, setActiveSheet]   = useState(0)
  const [loading, setLoading]           = useState(false)
  const [error, setError]               = useState(null)
  const [shareStatus, setShareStatus]   = useState('idle')
  const [markdownHtml, setMarkdownHtml] = useState('')
  const markdownRef                     = useRef(null)

  const handleShare = async () => {
    if (!filePath || shareStatus === 'sharing') return

    setShareStatus('sharing')
    try {
      const res = await apiFetch('/api/shares/gitnote', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          pathPrefix: filePath,
          title: decodePathSegment(filename),
        }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data?.error || 'Unable to share note')

      const encodedPath = encodePathSegments(filePath)
      const baseSharePath = data.token ? `/share/${encodeURIComponent(data.token)}` : data.url
      if (!baseSharePath) throw new Error('Share response missing URL')

      const sharePath = `${baseSharePath.replace(/\/$/, '')}/${encodedPath}`
      await copyToClipboard(window.location.origin + sharePath)
      setShareStatus('copied')
      setTimeout(() => setShareStatus('idle'), 2000)
    } catch {
      setShareStatus('failed')
      setTimeout(() => setShareStatus('idle'), 2000)
    }
  }

  const filename = filePath?.split('/').pop() || ''
  const ext      = filename.includes('.') ? filename.split('.').pop().toLowerCase() : 'txt'
  const isPdf    = ext === 'pdf'
  const isText   = TEXT_EXTENSIONS.has(ext)
  const isDocx   = DOCX_EXTENSIONS.has(ext)
  const isExcel  = EXCEL_EXTENSIONS.has(ext)
  const isImage  = IMAGE_EXTENSIONS.has(ext)
  const rawUrl   = filePath ? `${rawBase}?path=${encodeURIComponent(filePath)}` : null

  useEffect(() => {
    if (content === null || ext !== 'md') {
      setMarkdownHtml('')
      return
    }
    const raw = stripFrontMatter(content)
    const html = marked.parse(raw)
    const clean = typeof window !== 'undefined'
      ? DOMPurify.sanitize(html)
      : html
    setMarkdownHtml(rewriteImageSrcs(clean, filePath, rawBase))
  }, [content, ext, filePath, rawBase])

  useEffect(() => {
    const markdown = markdownRef.current
    if (!markdown || !markdownHtml) return

    markdown.querySelectorAll('pre > code').forEach(block => {
      const pre = block.parentElement
      if (pre && !pre.querySelector('.gn-copy-btn')) {
        const btn = document.createElement('button')
        btn.type = 'button'
        btn.className = 'gn-copy-btn'
        btn.title = 'Copy code'
        btn.setAttribute('aria-label', 'Copy code')
        btn.textContent = 'Copy'
        pre.appendChild(btn)
      }

      delete block.dataset.highlighted
      try {
        hljs.highlightElement(block)
      } catch (err) {
        block.classList.add('hljs')
        console.warn('Failed to highlight code block', err)
      }
    })

    const resetTimers = new Map()
    const resetButton = btn => {
      btn.textContent = 'Copy'
      btn.classList.remove('copied', 'failed')
    }
    const setButtonState = (btn, label, className) => {
      const currentTimer = resetTimers.get(btn)
      if (currentTimer) window.clearTimeout(currentTimer)

      btn.textContent = label
      btn.classList.remove('copied', 'failed')
      btn.classList.add(className)
      resetTimers.set(btn, window.setTimeout(() => {
        resetButton(btn)
        resetTimers.delete(btn)
      }, 2000))
    }

    const handleCopyCode = async event => {
      if (!(event.target instanceof Element)) return

      const btn = event.target.closest('.gn-copy-btn')
      if (!btn || !markdown.contains(btn)) return

      const code = btn.parentElement?.querySelector('code')
      if (!code) return

      try {
        await copyToClipboard(code.textContent || '')
        setButtonState(btn, 'Copied!', 'copied')
      } catch {
        setButtonState(btn, 'Failed', 'failed')
      }
    }

    markdown.addEventListener('click', handleCopyCode)
    return () => {
      markdown.removeEventListener('click', handleCopyCode)
      resetTimers.forEach(timer => window.clearTimeout(timer))
    }
  }, [markdownHtml])

  useEffect(() => {
    if (!filePath || isPdf) {
      setContent(null)
      setRendered(null)
      setError(null)
      return
    }

    setLoading(true)
    setContent(null)
    setRendered(null)
    setError(null)
    setActiveSheet(0)

    if (isText) {
      fetch(rawUrl)
        .then(r => {
          if (!r.ok) throw new Error(`Failed to load file (${r.status})`)
          return r.text()
        })
        .then(text => setContent(text))
        .catch(err => setError(err.message))
        .finally(() => setLoading(false))
      return
    }

    if (isDocx || isExcel) {
      fetch(rawUrl)
        .then(r => {
          if (!r.ok) throw new Error(`Failed to load file (${r.status})`)
          return r.arrayBuffer()
        })
        .then(async buffer => {
          if (isDocx) {
            const mammoth = await loadMammoth()
            const result = await mammoth.convertToHtml({ arrayBuffer: buffer })
            setRendered({ type: 'docx', html: result.value })
          } else {
            const mod = await import('xlsx')
            const XLSX = mod.default || mod
            const wb = XLSX.read(new Uint8Array(buffer), { type: 'array' })
            const sheets = wb.SheetNames.map(name => ({
              name,
              html: XLSX.utils.sheet_to_html(wb.Sheets[name]),
            }))
            setRendered({ type: 'excel', sheets })
          }
        })
        .catch(err => setError(err.message))
        .finally(() => setLoading(false))
      return
    }

    // Unknown binary — nothing to render
    setLoading(false)
  }, [filePath, rawUrl, isPdf, isText, isDocx, isExcel])

  if (!filePath) {
    return (
      <div className="gitnote-viewer-empty">
        <div className="gn-header-actions" style={{ position: 'absolute', top: '12px', right: '32px' }}>
          {onToggleTheme && (
            <button onClick={onToggleTheme} className="share-theme-btn" title="Toggle theme">
              {theme === 'dark' ? <Sun size={12} /> : <Moon size={12} />}
            </button>
          )}
          {onToggleExplorer && (
            <button onClick={onToggleExplorer} className="gn-share-btn gn-desktop-only" title="Toggle Files" style={{ marginLeft: 'auto' }}>
              <PanelRight size={12} />
              <span>{explorerVisible ? 'Hide Files' : 'Show Files'}</span>
            </button>
          )}
        </div>
        <FileText size={36} className="gn-empty-icon" />
        <p className="gn-empty-text">Select a file to view it</p>
      </div>
    )
  }

  return (
    <div className="gitnote-viewer">
      <div className="gitnote-viewer-header">
        {showBreadcrumbs && <Breadcrumb path={filePath} />}
        <div className="gn-header-actions">
          {!disableShare && (
            <button
              onClick={handleShare}
              className={`gn-share-btn ${shareStatus === 'copied' ? 'copied' : ''} ${shareStatus === 'failed' ? 'failed' : ''}`}
              title="Create public note link"
              disabled={shareStatus === 'sharing'}
            >
              {shareStatus === 'copied' ? <Check size={12} /> : <Share2 size={12} />}
              <span>
                {shareStatus === 'sharing' && 'Sharing...'}
                {shareStatus === 'copied' && 'Copied!'}
                {shareStatus === 'failed' && 'Failed'}
                {shareStatus === 'idle' && 'Share Note'}
              </span>
            </button>
          )}
          <a href={rawUrl} download={decodeURIComponent(filename)} className="gn-download-btn" title="Download file">
            <Download size={12} />
            <span>Download</span>
          </a>
          {onToggleTheme && (
            <button onClick={onToggleTheme} className="share-theme-btn" title="Toggle theme">
              {theme === 'dark' ? <Sun size={12} /> : <Moon size={12} />}
            </button>
          )}
          {onToggleExplorer && (
            <button onClick={onToggleExplorer} className="gn-share-btn gn-desktop-only" title="Toggle Files">
              <PanelRight size={12} />
              <span>{explorerVisible ? 'Hide Files' : 'Show Files'}</span>
            </button>
          )}
        </div>
      </div>

      <div className="gitnote-viewer-content">

        {/* PDF */}
        {isPdf && (
          <iframe src={rawUrl} className="gitnote-pdf-frame" title={filePath} />
        )}

        {/* Loading / error (shared) */}
        {!isPdf && loading && (
          <div className="gn-state-msg">
            <Loader2 size={15} className="animate-spin" />
            <span>Loading…</span>
          </div>
        )}
        {!isPdf && !loading && error && (
          <div className="gn-state-msg gn-state-error">
            <AlertCircle size={14} />
            <span>{error}</span>
          </div>
        )}

        {/* Markdown */}
        {!isPdf && !loading && !error && markdownHtml && (
          <div
            className="markdown-body"
            ref={markdownRef}
            dangerouslySetInnerHTML={{ __html: markdownHtml }}
          />
        )}

        {/* Plain text / code */}
        {!isPdf && !loading && !error && content !== null && ext !== 'md' && isText && (
          <pre className="gitnote-raw-text">{content}</pre>
        )}

        {/* DOCX */}
        {!isPdf && !loading && !error && rendered?.type === 'docx' && (
          <div className="gn-docx-body" dangerouslySetInnerHTML={{ __html: rendered.html }} />
        )}

        {/* Excel */}
        {!isPdf && !loading && !error && rendered?.type === 'excel' && (
          <div className="gn-excel-wrapper">
            {rendered.sheets.length > 1 && (
              <div className="gn-excel-tabs">
                {rendered.sheets.map((s, i) => (
                  <button
                    key={s.name}
                    className={`gn-excel-tab ${activeSheet === i ? 'active' : ''}`}
                    onClick={() => setActiveSheet(i)}
                  >
                    {s.name}
                  </button>
                ))}
              </div>
            )}
            <div
              className="gn-excel-table"
              dangerouslySetInnerHTML={{ __html: rendered.sheets[activeSheet]?.html }}
            />
          </div>
        )}

        {/* Image */}
        {isImage && (
          <div className="gn-image-view">
            <img src={rawUrl} alt={filename} className="gn-image-preview" />
          </div>
        )}

        {/* Unknown binary */}
        {!isPdf && !loading && !error && !isText && !isDocx && !isExcel && !isImage && (
          <div className="gn-state-msg" style={{ flexDirection: 'column', alignItems: 'flex-start', gap: '16px', paddingTop: '40px' }}>
            <span>Binary file — cannot display inline.</span>
            <a href={rawUrl} download className="gn-download-btn gn-download-prominent">
              <Download size={13} />
              Download file
            </a>
          </div>
        )}

      </div>
    </div>
  )
}
