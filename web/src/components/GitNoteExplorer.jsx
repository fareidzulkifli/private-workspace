'use client'

import { useState, useEffect } from 'react'
import { Folder, FolderOpen, FileText, File, Loader2, AlertCircle, FileCode, FileImage, FileSpreadsheet, FileArchive, FileAudio, FileVideo, FileType, Search } from 'lucide-react'

function isHidden(filePath) {
  const segments = filePath.split('/')
  return segments.some(segment => {
    const name = segment.toLowerCase()
    return name.startsWith('.')
      || name.includes('login')
      || name.endsWith('.rdp')
  })
}

function buildTree(files) {
  const root = {}
  for (const file of files) {
    const parts = file.path.split('/')
    let node = root
    for (let i = 0; i < parts.length - 1; i++) {
      const part = parts[i]
      if (!node[part]) node[part] = { __type: 'folder', __children: {} }
      node = node[part].__children
    }
    const name = parts[parts.length - 1]
    node[name] = { __type: 'file', __path: file.path, __size: file.size }
  }
  return root
}

function getFileIcon(name) {
  const ext = name.includes('.') ? name.split('.').pop().toLowerCase() : ''
  switch (ext) {
    case 'md':
    case 'txt':
      return <FileText size={13} className="gn-icon-md" />
    case 'doc':
    case 'docx':
    case 'rtf':
      return <FileType size={13} className="gn-icon-doc" />
    case 'js':
    case 'jsx':
    case 'ts':
    case 'tsx':
    case 'json':
    case 'html':
    case 'css':
    case 'py':
    case 'java':
    case 'c':
    case 'cpp':
    case 'go':
    case 'rs':
      return <FileCode size={13} className="gn-icon-code" />
    case 'png':
    case 'jpg':
    case 'jpeg':
    case 'gif':
    case 'svg':
    case 'webp':
    case 'ico':
      return <FileImage size={13} className="gn-icon-image" />
    case 'csv':
    case 'xls':
    case 'xlsx':
    case 'xlxs':
      return <FileSpreadsheet size={13} className="gn-icon-spreadsheet" />
    case 'zip':
    case 'tar':
    case 'gz':
    case 'rar':
    case '7z':
      return <FileArchive size={13} className="gn-icon-archive" />
    case 'mp3':
    case 'wav':
    case 'ogg':
      return <FileAudio size={13} className="gn-icon-audio" />
    case 'mp4':
    case 'webm':
    case 'mov':
      return <FileVideo size={13} className="gn-icon-video" />
    case 'pdf':
      return <File size={13} className="gn-icon-pdf" />
    default:
      return <File size={13} className="gn-icon-file" />
  }
}

function TreeNode({ name, node, depth, selectedPath, onSelect, defaultOpen }) {
  const [open, setOpen] = useState(defaultOpen)

  if (node.__type === 'file') {
    const isSelected = selectedPath === node.__path
    return (
      <div
        className={`gitnote-tree-item ${isSelected ? 'selected' : ''}`}
        style={{ paddingLeft: `${14 + depth * 14}px` }}
        onClick={() => onSelect(node.__path)}
        title={decodeURIComponent(name)}
      >
        {getFileIcon(name)}
        <span className="gitnote-tree-label">{decodeURIComponent(name)}</span>
      </div>
    )
  }

  return (
    <div>
      <div
        className="gitnote-tree-item gitnote-tree-folder"
        style={{ paddingLeft: `${14 + depth * 14}px` }}
        onClick={() => setOpen(o => !o)}
      >
        {open
          ? <FolderOpen size={13} className="gn-icon-folder" />
          : <Folder size={13} className="gn-icon-folder" />
        }
        <span className="gitnote-tree-label">{decodeURIComponent(name)}</span>
      </div>
      {open && (
        <div>
          {Object.entries(node.__children)
            .sort(([, a], [, b]) => {
              if (a.__type === b.__type) return 0
              return a.__type === 'folder' ? -1 : 1
            })
            .map(([childName, childNode]) => (
              <TreeNode
                key={childName}
                name={childName}
                node={childNode}
                depth={depth + 1}
                selectedPath={selectedPath}
                onSelect={onSelect}
                defaultOpen={false}
              />
            ))}
        </div>
      )}
    </div>
  )
}

export default function GitNoteExplorer({ selectedPath, onSelect, treeUrl = '/api/gitnote/tree', showSearch = true }) {
  const [rawFiles, setRawFiles] = useState(null)
  const [tree, setTree] = useState(null)
  const [error, setError] = useState(null)
  const [searchQuery, setSearchQuery] = useState('')

  useEffect(() => {
    fetch(treeUrl)
      .then(r => r.json())
      .then(data => {
        if (data.error) throw new Error(data.error)
        const files = data.tree.filter(f => !isHidden(f.path))
        setRawFiles(files)
        setTree(buildTree(files))
      })
      .catch(err => setError(err.message))
  }, [treeUrl])

  if (error) {
    return (
      <div className="gn-state-msg gn-state-error">
        <AlertCircle size={14} />
        <span>{error}</span>
      </div>
    )
  }

  if (!rawFiles || !tree) {
    return (
      <div className="gn-state-msg">
        <Loader2 size={14} className="animate-spin" />
        <span>Loading files…</span>
      </div>
    )
  }

  let content = null

  if (showSearch && searchQuery) {
    const q = searchQuery.toLowerCase()
    const matches = rawFiles.filter(f => {
      const name = f.path.split('/').pop().toLowerCase()
      const ext = name.includes('.') ? name.split('.').pop() : ''
      // Only search inside .txt, .docx, .md, .pdf as requested
      const isSearchableType = ['txt', 'docx', 'md', 'pdf'].includes(ext)
      if (!isSearchableType) return false
      return name.includes(q)
    })

    content = (
      <div className="gitnote-search-results">
        {matches.map(f => {
          const name = f.path.split('/').pop()
          return (
            <div
              key={f.path}
              className={`gitnote-tree-item ${selectedPath === f.path ? 'selected' : ''}`}
              style={{ paddingLeft: '14px', alignItems: 'flex-start', padding: '8px 14px' }}
              onClick={() => onSelect(f.path)}
              title={f.path}
            >
              <div style={{ marginTop: '2px' }}>{getFileIcon(name)}</div>
              <div style={{ display: 'flex', flexDirection: 'column', overflow: 'hidden', marginLeft: '6px' }}>
                <span className="gitnote-tree-label" style={{ fontSize: '13px' }}>{decodeURIComponent(name)}</span>
                <span style={{ fontSize: '11px', color: 'var(--gn-text-3)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {decodeURIComponent(f.path)}
                </span>
              </div>
            </div>
          )
        })}
        {matches.length === 0 && (
          <div className="gn-state-msg">No matching documents found.</div>
        )}
      </div>
    )
  } else {
    content = (
      <div className="gitnote-tree-root">
        {Object.entries(tree)
          .sort(([, a], [, b]) => {
            if (a.__type === b.__type) return 0
            return a.__type === 'folder' ? -1 : 1
          })
          .map(([name, node]) => (
            <TreeNode
              key={name}
              name={name}
              node={node}
              depth={0}
              selectedPath={selectedPath}
              onSelect={onSelect}
              defaultOpen={false}
            />
          ))}
      </div>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      {showSearch && (
        <div style={{ padding: '10px 14px', borderBottom: '1px solid var(--gn-border)' }}>
          <div className="gn-search-box">
            <Search size={14} className="gn-search-icon" />
            <input
              type="text"
              placeholder="Search .md, .pdf, .docx..."
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              className="gn-search-input"
            />
          </div>
        </div>
      )}
      <div style={{ flex: 1, overflowY: 'auto' }}>
        {content}
      </div>
    </div>
  )
}
