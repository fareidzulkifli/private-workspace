'use client'

import { useEffect, useMemo, useState } from 'react'
import {
  Archive,
  ArchiveRestore,
  Boxes,
  ChevronDown,
  ChevronUp,
  Copy,
  FileText,
  MessageSquareText,
  Plus,
  Save,
  Search,
  Star,
  X
} from 'lucide-react'
import { buildContextText, estimateTokens, extractVariables, renderPrompt } from '@/lib/prompts/renderPrompt'

const blankPrompt = {
  title: '',
  category: 'General',
  tags: '',
  body: '',
  is_favorite: false,
  archived_at: null,
}

const blankPack = {
  title: '',
  tags: '',
  archived_at: null,
  items: [],
}

const parseTags = (value = '') =>
  value
    .split(',')
    .map(tag => tag.trim())
    .filter(Boolean)

const tagsToInput = (tags) => Array.isArray(tags) ? tags.join(', ') : ''

const makeDraftItem = () => ({
  client_id: `draft-${Date.now()}-${Math.random().toString(16).slice(2)}`,
  title: '',
  body: '',
  sort_order: 0,
  enabled_by_default: true,
})

const toPromptDraft = (prompt = blankPrompt) => ({
  ...blankPrompt,
  ...prompt,
  tags: tagsToInput(prompt.tags),
})

const toPackDraft = (pack = blankPack) => ({
  ...blankPack,
  ...pack,
  tags: tagsToInput(pack.tags),
  items: (pack.items || []).map((item, index) => ({
    ...item,
    sort_order: item.sort_order ?? index,
    enabled_by_default: item.enabled_by_default !== false,
  })),
})

const sortByUpdated = (items) =>
  [...items].sort((a, b) => {
    if (!!a.archived_at !== !!b.archived_at) return a.archived_at ? 1 : -1
    if (!!a.is_favorite !== !!b.is_favorite) return a.is_favorite ? -1 : 1
    return new Date(b.updated_at || b.created_at || 0) - new Date(a.updated_at || a.created_at || 0)
  })

const filterBySearch = (items, query, showArchived) => {
  const q = query.trim().toLowerCase()
  return items.filter(item => {
    if (!showArchived && item.archived_at) return false
    if (!q) return true
    return [
      item.title,
      item.category,
      ...(Array.isArray(item.tags) ? item.tags : []),
    ].filter(Boolean).join(' ').toLowerCase().includes(q)
  })
}

export default function PromptVault() {
  const [activeTab, setActiveTab] = useState('prompts')
  const [prompts, setPrompts] = useState([])
  const [contextPacks, setContextPacks] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [saveStatus, setSaveStatus] = useState('')
  const [promptSearch, setPromptSearch] = useState('')
  const [packSearch, setPackSearch] = useState('')
  const [showArchivedPrompts, setShowArchivedPrompts] = useState(false)
  const [showArchivedPacks, setShowArchivedPacks] = useState(false)
  const [activePromptId, setActivePromptId] = useState(null)
  const [activePackId, setActivePackId] = useState(null)
  const [promptDraft, setPromptDraft] = useState(blankPrompt)
  const [packDraft, setPackDraft] = useState(blankPack)
  const [usePrompt, setUsePrompt] = useState(null)
  const [useValues, setUseValues] = useState({})
  const [useContextPackId, setUseContextPackId] = useState('')
  const [enabledContextIds, setEnabledContextIds] = useState(new Set())
  const [copyStatus, setCopyStatus] = useState('')

  const activePacks = useMemo(
    () => contextPacks.filter(pack => !pack.archived_at),
    [contextPacks]
  )

  const selectedContextPack = useMemo(
    () => contextPacks.find(pack => pack.id === useContextPackId),
    [contextPacks, useContextPackId]
  )

  const selectedContextItems = useMemo(
    () => selectedContextPack?.items || [],
    [selectedContextPack]
  )

  const contextText = useMemo(
    () => buildContextText(selectedContextItems, enabledContextIds),
    [selectedContextItems, enabledContextIds]
  )

  const useVariables = useMemo(
    () => extractVariables(usePrompt?.body || ''),
    [usePrompt]
  )

  const visibleVariables = useMemo(
    () => useVariables.filter(variable => !(variable === 'context' && contextText)),
    [useVariables, contextText]
  )

  const renderedPrompt = useMemo(
    () => renderPrompt({
      body: usePrompt?.body || '',
      values: useValues,
      contextText,
    }),
    [usePrompt, useValues, contextText]
  )

  const filteredPrompts = useMemo(
    () => filterBySearch(sortByUpdated(prompts), promptSearch, showArchivedPrompts),
    [prompts, promptSearch, showArchivedPrompts]
  )

  const filteredPacks = useMemo(
    () => filterBySearch(sortByUpdated(contextPacks), packSearch, showArchivedPacks),
    [contextPacks, packSearch, showArchivedPacks]
  )

  const activePrompt = useMemo(
    () => prompts.find(prompt => prompt.id === activePromptId),
    [prompts, activePromptId]
  )

  const activePack = useMemo(
    () => contextPacks.find(pack => pack.id === activePackId),
    [contextPacks, activePackId]
  )

  const loadData = async () => {
    try {
      setLoading(true)
      setError('')
      const [promptRes, packRes] = await Promise.all([
        fetch('/api/prompts/templates?include_archived=true'),
        fetch('/api/prompts/context-packs?include_archived=true'),
      ])
      const [promptData, packData] = await Promise.all([
        promptRes.json(),
        packRes.json(),
      ])

      if (!promptRes.ok) throw new Error(promptData.error || 'Unable to load prompts')
      if (!packRes.ok) throw new Error(packData.error || 'Unable to load context packs')

      setPrompts(promptData)
      setContextPacks(packData)

      const firstPrompt = promptData.find(prompt => !prompt.archived_at) || promptData[0]
      const firstPack = packData.find(pack => !pack.archived_at) || packData[0]

      if (firstPrompt && !activePromptId) {
        setActivePromptId(firstPrompt.id)
        setPromptDraft(toPromptDraft(firstPrompt))
      }

      if (firstPack && !activePackId) {
        setActivePackId(firstPack.id)
        setPackDraft(toPackDraft(firstPack))
      }
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadData()
  }, [])

  useEffect(() => {
    if (!useContextPackId) {
      setEnabledContextIds(new Set())
      return
    }

    const pack = contextPacks.find(item => item.id === useContextPackId)
    setEnabledContextIds(new Set((pack?.items || [])
      .filter(item => item.enabled_by_default !== false)
      .map(item => item.id)))
  }, [useContextPackId, contextPacks])

  const selectPrompt = (prompt) => {
    setActivePromptId(prompt.id)
    setPromptDraft(toPromptDraft(prompt))
    setSaveStatus('')
  }

  const selectPack = (pack) => {
    setActivePackId(pack.id)
    setPackDraft(toPackDraft(pack))
    setSaveStatus('')
  }

  const startNewPrompt = () => {
    setActivePromptId(null)
    setPromptDraft(blankPrompt)
    setSaveStatus('')
  }

  const startNewPack = () => {
    setActivePackId(null)
    setPackDraft({
      ...blankPack,
      items: [makeDraftItem()],
    })
    setSaveStatus('')
  }

  const savePrompt = async () => {
    try {
      setSaveStatus('Saving...')
      const payload = {
        title: promptDraft.title,
        category: promptDraft.category,
        tags: parseTags(promptDraft.tags),
        body: promptDraft.body,
        is_favorite: promptDraft.is_favorite,
      }
      const res = await fetch(activePromptId ? `/api/prompts/templates/${activePromptId}` : '/api/prompts/templates', {
        method: activePromptId ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || 'Unable to save prompt')

      setPrompts(prev => activePromptId
        ? prev.map(prompt => prompt.id === data.id ? data : prompt)
        : [data, ...prev])
      setActivePromptId(data.id)
      setPromptDraft(toPromptDraft(data))
      setSaveStatus('Saved')
    } catch (err) {
      setSaveStatus(err.message)
    }
  }

  const togglePromptArchive = async () => {
    if (!activePromptId) return
    try {
      setSaveStatus(activePrompt?.archived_at ? 'Restoring...' : 'Archiving...')
      const res = await fetch(`/api/prompts/templates/${activePromptId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ archived: !activePrompt?.archived_at }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || 'Unable to update prompt')

      setPrompts(prev => prev.map(prompt => prompt.id === data.id ? data : prompt))
      setPromptDraft(toPromptDraft(data))
      setSaveStatus(data.archived_at ? 'Archived' : 'Restored')
    } catch (err) {
      setSaveStatus(err.message)
    }
  }

  const savePack = async () => {
    try {
      setSaveStatus('Saving...')
      const payload = {
        title: packDraft.title,
        tags: parseTags(packDraft.tags),
        items: (packDraft.items || []).map((item, index) => ({
          title: item.title,
          body: item.body,
          enabled_by_default: item.enabled_by_default !== false,
          sort_order: index,
        })),
      }
      const res = await fetch(activePackId ? `/api/prompts/context-packs/${activePackId}` : '/api/prompts/context-packs', {
        method: activePackId ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || 'Unable to save context pack')

      setContextPacks(prev => activePackId
        ? prev.map(pack => pack.id === data.id ? data : pack)
        : [data, ...prev])
      setActivePackId(data.id)
      setPackDraft(toPackDraft(data))
      setSaveStatus('Saved')
    } catch (err) {
      setSaveStatus(err.message)
    }
  }

  const togglePackArchive = async () => {
    if (!activePackId) return
    try {
      setSaveStatus(activePack?.archived_at ? 'Restoring...' : 'Archiving...')
      const res = await fetch(`/api/prompts/context-packs/${activePackId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ archived: !activePack?.archived_at }),
      })
      const data = await res.json()
      if (!res.ok) throw new Error(data.error || 'Unable to update context pack')

      setContextPacks(prev => prev.map(pack => pack.id === data.id ? data : pack))
      setPackDraft(toPackDraft(data))
      setSaveStatus(data.archived_at ? 'Archived' : 'Restored')
    } catch (err) {
      setSaveStatus(err.message)
    }
  }

  const updatePackItem = (key, updates) => {
    setPackDraft(prev => ({
      ...prev,
      items: prev.items.map(item => (item.id || item.client_id) === key ? { ...item, ...updates } : item),
    }))
  }

  const movePackItem = (key, direction) => {
    setPackDraft(prev => {
      const items = [...prev.items]
      const index = items.findIndex(item => (item.id || item.client_id) === key)
      const nextIndex = index + direction
      if (index < 0 || nextIndex < 0 || nextIndex >= items.length) return prev
      const [item] = items.splice(index, 1)
      items.splice(nextIndex, 0, item)
      return { ...prev, items }
    })
  }

  const removePackItem = (key) => {
    setPackDraft(prev => ({
      ...prev,
      items: prev.items.filter(item => (item.id || item.client_id) !== key),
    }))
  }

  const openUsePrompt = (prompt) => {
    setUsePrompt(prompt)
    setUseValues({})
    setUseContextPackId('')
    setEnabledContextIds(new Set())
    setCopyStatus('')
  }

  const copyRenderedPrompt = async () => {
    try {
      await navigator.clipboard.writeText(renderedPrompt)
      setCopyStatus('Copied')
    } catch {
      setCopyStatus('Copy failed')
    }
  }

  return (
    <div className="prompt-page">
      <header className="prompt-header">
        <div className="prompt-header-left">
          <button
            className="btn-ghost prompt-mobile-menu"
            onClick={() => window.dispatchEvent(new Event('toggle-sidebar'))}
            aria-label="Open navigation"
          >
            <ChevronDown size={16} />
          </button>
          <div className="prompt-title-mark">
            <MessageSquareText size={18} />
          </div>
          <div>
            <h1>Prompt Studio</h1>
            <p>Reusable prompts and lightweight context packs.</p>
          </div>
        </div>

        <div className="prompt-tabs" role="tablist" aria-label="Prompt studio sections">
          <button
            className={activeTab === 'prompts' ? 'is-active' : ''}
            onClick={() => setActiveTab('prompts')}
          >
            <MessageSquareText size={14} />
            <span>Prompts</span>
          </button>
          <button
            className={activeTab === 'context' ? 'is-active' : ''}
            onClick={() => setActiveTab('context')}
          >
            <Boxes size={14} />
            <span>Context Packs</span>
          </button>
        </div>
      </header>

      {error && (
        <div className="prompt-error">
          {error}
        </div>
      )}

      {loading ? (
        <div className="prompt-loading">Loading prompt studio...</div>
      ) : activeTab === 'prompts' ? (
        <section className="prompt-workspace">
          <aside className="prompt-rail">
            <div className="prompt-rail-tools">
              <div className="prompt-search">
                <Search size={14} />
                <input
                  value={promptSearch}
                  onChange={(event) => setPromptSearch(event.target.value)}
                  placeholder="Search prompts"
                />
              </div>
              <button className="btn-primary prompt-compact-btn" onClick={startNewPrompt}>
                <Plus size={14} />
                <span>New</span>
              </button>
            </div>

            <button
              className={`prompt-archive-toggle ${showArchivedPrompts ? 'is-active' : ''}`}
              onClick={() => setShowArchivedPrompts(prev => !prev)}
            >
              <Archive size={13} />
              <span>{showArchivedPrompts ? 'Hide archived' : 'Show archived'}</span>
            </button>

            <div className="prompt-list">
              {filteredPrompts.map(prompt => (
                <button
                  key={prompt.id}
                  className={`prompt-list-row ${activePromptId === prompt.id ? 'is-selected' : ''} ${prompt.archived_at ? 'is-archived' : ''}`}
                  onClick={() => selectPrompt(prompt)}
                >
                  <span className="prompt-list-icon">
                    {prompt.is_favorite ? <Star size={13} fill="currentColor" /> : <FileText size={13} />}
                  </span>
                  <span className="prompt-list-copy">
                    <strong>{prompt.title}</strong>
                    <small>{prompt.category || 'General'} · {extractVariables(prompt.body).length} vars</small>
                  </span>
                  <span className="prompt-list-use" onClick={(event) => { event.stopPropagation(); openUsePrompt(prompt) }}>
                    Use
                  </span>
                </button>
              ))}

              {filteredPrompts.length === 0 && (
                <div className="prompt-empty">
                  No prompts found.
                </div>
              )}
            </div>
          </aside>

          <PromptEditor
            draft={promptDraft}
            isNew={!activePromptId}
            selected={activePrompt}
            saveStatus={saveStatus}
            onChange={setPromptDraft}
            onSave={savePrompt}
            onUse={() => activePrompt ? openUsePrompt(activePrompt) : null}
            onArchive={togglePromptArchive}
          />
        </section>
      ) : (
        <section className="prompt-workspace">
          <aside className="prompt-rail">
            <div className="prompt-rail-tools">
              <div className="prompt-search">
                <Search size={14} />
                <input
                  value={packSearch}
                  onChange={(event) => setPackSearch(event.target.value)}
                  placeholder="Search context"
                />
              </div>
              <button className="btn-primary prompt-compact-btn" onClick={startNewPack}>
                <Plus size={14} />
                <span>New</span>
              </button>
            </div>

            <button
              className={`prompt-archive-toggle ${showArchivedPacks ? 'is-active' : ''}`}
              onClick={() => setShowArchivedPacks(prev => !prev)}
            >
              <Archive size={13} />
              <span>{showArchivedPacks ? 'Hide archived' : 'Show archived'}</span>
            </button>

            <div className="prompt-list">
              {filteredPacks.map(pack => (
                <button
                  key={pack.id}
                  className={`prompt-list-row ${activePackId === pack.id ? 'is-selected' : ''} ${pack.archived_at ? 'is-archived' : ''}`}
                  onClick={() => selectPack(pack)}
                >
                  <span className="prompt-list-icon">
                    <Boxes size={13} />
                  </span>
                  <span className="prompt-list-copy">
                    <strong>{pack.title}</strong>
                    <small>{(pack.items || []).length} items</small>
                  </span>
                </button>
              ))}

              {filteredPacks.length === 0 && (
                <div className="prompt-empty">
                  No context packs found.
                </div>
              )}
            </div>
          </aside>

          <ContextPackEditor
            draft={packDraft}
            isNew={!activePackId}
            selected={activePack}
            saveStatus={saveStatus}
            onChange={setPackDraft}
            onSave={savePack}
            onArchive={togglePackArchive}
            onUpdateItem={updatePackItem}
            onMoveItem={movePackItem}
            onRemoveItem={removePackItem}
          />
        </section>
      )}

      {usePrompt && (
        <UsePromptModal
          prompt={usePrompt}
          activePacks={activePacks}
          selectedContextPack={selectedContextPack}
          useContextPackId={useContextPackId}
          setUseContextPackId={setUseContextPackId}
          enabledContextIds={enabledContextIds}
          setEnabledContextIds={setEnabledContextIds}
          visibleVariables={visibleVariables}
          useValues={useValues}
          setUseValues={setUseValues}
          renderedPrompt={renderedPrompt}
          copyStatus={copyStatus}
          onCopy={copyRenderedPrompt}
          onClose={() => setUsePrompt(null)}
        />
      )}
    </div>
  )
}

function PromptEditor({ draft, isNew, selected, saveStatus, onChange, onSave, onUse, onArchive }) {
  const variables = extractVariables(draft.body)

  return (
    <main className="prompt-editor">
      <div className="prompt-editor-header">
        <div>
          <span className="prompt-eyebrow">{isNew ? 'New prompt' : selected?.archived_at ? 'Archived prompt' : 'Prompt template'}</span>
          <h2>{draft.title || 'Untitled prompt'}</h2>
        </div>
        <div className="prompt-editor-actions">
          {saveStatus && <span className="prompt-save-status">{saveStatus}</span>}
          {!isNew && (
            <button className="btn-ghost prompt-compact-btn" onClick={onUse} disabled={!!selected?.archived_at}>
              <Copy size={14} />
              <span>Use</span>
            </button>
          )}
          {!isNew && (
            <button className="btn-ghost prompt-compact-btn" onClick={onArchive}>
              {selected?.archived_at ? <ArchiveRestore size={14} /> : <Archive size={14} />}
              <span>{selected?.archived_at ? 'Restore' : 'Archive'}</span>
            </button>
          )}
          <button className="btn-primary prompt-compact-btn" onClick={onSave}>
            <Save size={14} />
            <span>Save</span>
          </button>
        </div>
      </div>

      <div className="prompt-form-grid">
        <label className="prompt-field prompt-field-wide">
          <span>Title</span>
          <input
            value={draft.title}
            onChange={(event) => onChange({ ...draft, title: event.target.value })}
            placeholder="Strategy review"
          />
        </label>

        <label className="prompt-field">
          <span>Category</span>
          <input
            value={draft.category}
            onChange={(event) => onChange({ ...draft, category: event.target.value })}
            placeholder="Strategy"
          />
        </label>

        <label className="prompt-field">
          <span>Tags</span>
          <input
            value={draft.tags}
            onChange={(event) => onChange({ ...draft, tags: event.target.value })}
            placeholder="review, product"
          />
        </label>
      </div>

      <label className="prompt-favorite">
        <input
          type="checkbox"
          checked={draft.is_favorite}
          onChange={(event) => onChange({ ...draft, is_favorite: event.target.checked })}
        />
        <span>Favorite this prompt</span>
      </label>

      <label className="prompt-field prompt-body-field">
        <span>Prompt Body</span>
        <textarea
          value={draft.body}
          onChange={(event) => onChange({ ...draft, body: event.target.value })}
          placeholder={'Review {topic} using this context:\n\n{context}\n\nReturn risks, options, and a recommended next move.'}
        />
      </label>

      <div className="prompt-editor-foot">
        <div className="prompt-variable-strip">
          <strong>{variables.length}</strong>
          <span>{variables.length === 1 ? 'variable detected' : 'variables detected'}</span>
          {variables.map(variable => (
            <code key={variable}>{variable}</code>
          ))}
        </div>
      </div>
    </main>
  )
}

function ContextPackEditor({ draft, isNew, selected, saveStatus, onChange, onSave, onArchive, onUpdateItem, onMoveItem, onRemoveItem }) {
  const addItem = () => {
    onChange({
      ...draft,
      items: [...draft.items, makeDraftItem()],
    })
  }

  return (
    <main className="prompt-editor">
      <div className="prompt-editor-header">
        <div>
          <span className="prompt-eyebrow">{isNew ? 'New context pack' : selected?.archived_at ? 'Archived context pack' : 'Context pack'}</span>
          <h2>{draft.title || 'Untitled context pack'}</h2>
        </div>
        <div className="prompt-editor-actions">
          {saveStatus && <span className="prompt-save-status">{saveStatus}</span>}
          {!isNew && (
            <button className="btn-ghost prompt-compact-btn" onClick={onArchive}>
              {selected?.archived_at ? <ArchiveRestore size={14} /> : <Archive size={14} />}
              <span>{selected?.archived_at ? 'Restore' : 'Archive'}</span>
            </button>
          )}
          <button className="btn-primary prompt-compact-btn" onClick={onSave}>
            <Save size={14} />
            <span>Save</span>
          </button>
        </div>
      </div>

      <div className="prompt-form-grid">
        <label className="prompt-field prompt-field-wide">
          <span>Title</span>
          <input
            value={draft.title}
            onChange={(event) => onChange({ ...draft, title: event.target.value })}
            placeholder="Task Manager Product Context"
          />
        </label>

        <label className="prompt-field prompt-field-wide">
          <span>Tags</span>
          <input
            value={draft.tags}
            onChange={(event) => onChange({ ...draft, tags: event.target.value })}
            placeholder="product, design, frontend"
          />
        </label>
      </div>

      <div className="context-items-header">
        <div>
          <span>Context Items</span>
          <small>{draft.items.length} reusable blocks</small>
        </div>
        <button className="btn-ghost prompt-compact-btn" onClick={addItem}>
          <Plus size={14} />
          <span>Add item</span>
        </button>
      </div>

      <div className="context-items">
        {draft.items.map((item, index) => {
          const key = item.id || item.client_id
          return (
            <div className="context-item-editor" key={key}>
              <div className="context-item-toolbar">
                <label className="prompt-favorite context-item-toggle">
                  <input
                    type="checkbox"
                    checked={item.enabled_by_default !== false}
                    onChange={(event) => onUpdateItem(key, { enabled_by_default: event.target.checked })}
                  />
                  <span>Include by default</span>
                </label>
                <div className="context-item-buttons">
                  <button className="btn-ghost" onClick={() => onMoveItem(key, -1)} disabled={index === 0} aria-label="Move item up">
                    <ChevronUp size={14} />
                  </button>
                  <button className="btn-ghost" onClick={() => onMoveItem(key, 1)} disabled={index === draft.items.length - 1} aria-label="Move item down">
                    <ChevronDown size={14} />
                  </button>
                  <button className="btn-ghost" onClick={() => onRemoveItem(key)} aria-label="Remove context item">
                    <X size={14} />
                  </button>
                </div>
              </div>

              <label className="prompt-field">
                <span>Item title</span>
                <input
                  value={item.title}
                  onChange={(event) => onUpdateItem(key, { title: event.target.value })}
                  placeholder="Design preferences"
                />
              </label>

              <label className="prompt-field">
                <span>Context</span>
                <textarea
                  value={item.body}
                  onChange={(event) => onUpdateItem(key, { body: event.target.value })}
                  placeholder="Compact UI, professional enterprise aesthetic, avoid generic decoration..."
                />
              </label>
            </div>
          )
        })}

        {draft.items.length === 0 && (
          <div className="prompt-empty context-empty">
            Add one context item to make this pack useful.
          </div>
        )}
      </div>
    </main>
  )
}

function UsePromptModal({
  prompt,
  activePacks,
  selectedContextPack,
  useContextPackId,
  setUseContextPackId,
  enabledContextIds,
  setEnabledContextIds,
  visibleVariables,
  useValues,
  setUseValues,
  renderedPrompt,
  copyStatus,
  onCopy,
  onClose,
}) {
  const toggleContextItem = (id) => {
    setEnabledContextIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  return (
    <div className="modal-overlay prompt-use-overlay">
      <div className="prompt-use-modal">
        <header className="prompt-use-header">
          <div>
            <span className="prompt-eyebrow">Use prompt</span>
            <h2>{prompt.title}</h2>
          </div>
          <button className="btn-ghost" onClick={onClose} aria-label="Close prompt composer">
            <X size={16} />
          </button>
        </header>

        <div className="prompt-use-body">
          <section className="prompt-use-controls">
            <label className="prompt-field">
              <span>Context pack</span>
              <select
                value={useContextPackId}
                onChange={(event) => setUseContextPackId(event.target.value)}
              >
                <option value="">No context pack</option>
                {activePacks.map(pack => (
                  <option key={pack.id} value={pack.id}>{pack.title}</option>
                ))}
              </select>
            </label>

            {selectedContextPack && (
              <div className="prompt-context-picker">
                <span>Included context</span>
                {(selectedContextPack.items || []).map(item => (
                  <label key={item.id} className="prompt-context-toggle">
                    <input
                      type="checkbox"
                      checked={enabledContextIds.has(item.id)}
                      onChange={() => toggleContextItem(item.id)}
                    />
                    <small>{item.title}</small>
                  </label>
                ))}
              </div>
            )}

            <div className="prompt-variable-fields">
              {visibleVariables.map(variable => (
                <label className="prompt-field" key={variable}>
                  <span>{variable}</span>
                  <textarea
                    value={useValues[variable] || ''}
                    onChange={(event) => setUseValues(prev => ({ ...prev, [variable]: event.target.value }))}
                    placeholder={`Value for {${variable}}`}
                  />
                </label>
              ))}

              {visibleVariables.length === 0 && (
                <div className="prompt-empty">
                  No fields to fill. Review the preview and copy.
                </div>
              )}
            </div>
          </section>

          <section className="prompt-preview-panel">
            <div className="prompt-preview-header">
              <div>
                <span>Final prompt</span>
                <small>{estimateTokens(renderedPrompt).toLocaleString()} estimated tokens</small>
              </div>
              <button className="btn-primary prompt-compact-btn" onClick={onCopy} disabled={!renderedPrompt}>
                <Copy size={14} />
                <span>{copyStatus || 'Copy'}</span>
              </button>
            </div>
            <pre>{renderedPrompt || 'The final prompt will appear here.'}</pre>
          </section>
        </div>
      </div>
    </div>
  )
}
