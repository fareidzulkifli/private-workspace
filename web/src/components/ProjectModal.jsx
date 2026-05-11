'use client';

import { useState, useEffect, useRef } from 'react'
import { X, Loader2, Archive } from 'lucide-react'

export default function ProjectModal({ project, onClose, onProjectUpdate, onProjectArchived, pendingTaskCount = 0 }) {
  const [edited, setEdited] = useState(project)
  const [isSaving, setIsSaving] = useState(false)
  const [previewMarkdown, setPreviewMarkdown] = useState(false)
  const [isMobile, setIsMobile] = useState(false)
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false)
  const nameRef = useRef(null)
  const canArchive = pendingTaskCount === 0

  useEffect(() => {
    setEdited(project)
  }, [project])

  useEffect(() => {
    if (nameRef.current) {
      nameRef.current.style.height = 'auto'
      nameRef.current.style.height = `${nameRef.current.scrollHeight}px`
    }
  }, [edited.name])

  useEffect(() => {
    const checkMobile = () => setIsMobile(window.innerWidth <= 768)
    checkMobile()
    window.addEventListener('resize', checkMobile)
    return () => window.removeEventListener('resize', checkMobile)
  }, [])

  const save = async (data = edited) => {
    try {
      setIsSaving(true)
      const res = await fetch(`/api/projects/${project.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: data.name,
          goal: data.goal,
          description_markdown: data.description_markdown
        })
      })
      const result = await res.json()
      if (result.error) throw new Error(result.error)
      setEdited(result)
      onProjectUpdate?.(result)
      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Error saving project: ' + err.message)
    } finally {
      setIsSaving(false)
    }
  }

  const archiveProject = async () => {
    if (!canArchive) return

    try {
      setIsSaving(true)
      const res = await fetch(`/api/projects/${project.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ archived: true })
      })
      const result = await res.json()
      if (result.error) throw new Error(result.error)
      setEdited(result)
      onProjectArchived?.(result)
      setShowArchiveConfirm(false)
      window.dispatchEvent(new Event('taskUpdated'))
      onClose()
    } catch (err) {
      alert('Error archiving project: ' + err.message)
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <div
      className="modal-overlay"
      style={{ alignItems: isMobile ? 'center' : 'flex-start', paddingTop: isMobile ? '0' : '4vh' }}
      onClick={(e) => e.target.classList.contains('modal-overlay') && onClose()}
      onKeyDown={(e) => e.key === 'Escape' && onClose()}
    >
      <div
        className="modal-content"
        style={{
          maxWidth: '680px',
          width: '100%',
          overflow: 'hidden',
          display: 'flex',
          flexDirection: 'column',
          height: isMobile ? '100vh' : 'auto',
          maxHeight: isMobile ? '100vh' : '85vh',
          borderRadius: isMobile ? '0' : 'var(--radius-xl)',
          background: 'var(--background)',
          boxShadow: 'var(--shadow-xl)',
          borderLeft: '1px solid var(--border-strong)',
          borderRight: '1px solid var(--border-strong)',
          borderBottom: '1px solid var(--border-strong)',
          borderTop: '2px solid var(--accent)',
        }}
      >
        {/* Top Bar */}
        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 20px',
          background: 'var(--surface)',
          borderBottom: '1px solid var(--border)',
          height: '48px',
          flexShrink: 0
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <span style={{
              fontSize: '11px',
              fontWeight: '700',
              color: 'var(--text-disabled)',
              letterSpacing: '0.08em',
              fontFamily: 'var(--font-mono)'
            }}>
              PROJECT DETAILS
            </span>
            {isSaving && (
              <>
                <div style={{ width: '1px', height: '12px', background: 'var(--border-strong)' }} />
                <Loader2 className="animate-spin" size={12} color="var(--accent)" />
              </>
            )}
          </div>
          <button onClick={onClose} className="btn-ghost" style={{ padding: '6px', borderRadius: '4px' }}>
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div style={{
          overflowY: 'auto',
          padding: isMobile ? '20px' : '32px 40px',
          display: 'flex',
          flexDirection: 'column',
          gap: '28px',
          flexGrow: 1
        }}>
          {/* Name */}
          <textarea
            ref={nameRef}
            value={edited.name}
            onChange={e => {
              setEdited({ ...edited, name: e.target.value })
              e.target.style.height = 'auto'
              e.target.style.height = `${e.target.scrollHeight}px`
            }}
            onBlur={() => save()}
            rows={1}
            style={{
              fontSize: isMobile ? '20px' : '26px',
              fontWeight: '700',
              background: 'transparent',
              border: 'none',
              padding: '0',
              boxShadow: 'none',
              letterSpacing: '-0.03em',
              color: 'var(--text)',
              width: '100%',
              flexShrink: 0,
              resize: 'none',
              overflow: 'hidden',
              lineHeight: '1.3',
            }}
          />

          {/* Goal */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
            <span style={{
              fontSize: '10px',
              fontWeight: '800',
              color: 'var(--text-disabled)',
              textTransform: 'uppercase',
              letterSpacing: '0.12em'
            }}>
              Goal
            </span>
            <input
              type="text"
              value={edited.goal || ''}
              onChange={e => setEdited({ ...edited, goal: e.target.value })}
              onBlur={() => save()}
              placeholder="Why did you create this project?"
              style={{
                background: 'transparent',
                border: 'none',
                borderBottom: '1px solid var(--border)',
                borderRadius: '0',
                padding: '8px 0',
                fontSize: '14px',
                color: 'var(--text-secondary)',
                boxShadow: 'none',
              }}
            />
          </div>

          {/* Notes */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', flexGrow: 1 }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <span style={{
                fontSize: '10px',
                fontWeight: '800',
                color: 'var(--text-disabled)',
                textTransform: 'uppercase',
                letterSpacing: '0.12em'
              }}>
                Notes
              </span>
              <button
                onClick={() => setPreviewMarkdown(!previewMarkdown)}
                className="btn-ghost"
                style={{ fontSize: '11px', padding: '4px 10px' }}
              >
                {previewMarkdown ? 'Edit' : 'Preview'}
              </button>
            </div>
            {!previewMarkdown ? (
              <textarea
                value={edited.description_markdown || ''}
                onChange={e => setEdited({ ...edited, description_markdown: e.target.value })}
                onBlur={() => save()}
                placeholder="Add notes about this project..."
                style={{
                  minHeight: isMobile ? 'calc(100vh - 380px)' : '240px',
                  background: 'transparent',
                  border: 'none',
                  padding: '0',
                  resize: 'none',
                  lineHeight: '1.75',
                  fontSize: '14px',
                  color: 'var(--text-secondary)',
                  fontFamily: 'Inter, sans-serif'
                }}
              />
            ) : (
              <div style={{
                lineHeight: '1.8',
                fontSize: '14px',
                color: 'var(--text-secondary)',
                whiteSpace: 'pre-wrap',
                minHeight: '240px',
              }}>
                {edited.description_markdown || 'Nothing written yet.'}
              </div>
            )}
          </div>

          {!edited.archived_at && (
            <div style={{
              borderTop: '1px solid var(--border)',
              paddingTop: '20px',
              display: 'flex',
              alignItems: isMobile ? 'stretch' : 'center',
              justifyContent: 'space-between',
              gap: '12px',
              flexDirection: isMobile ? 'column' : 'row'
            }}>
              <div>
                <div style={{
                  fontSize: '10px',
                  fontWeight: '800',
                  color: 'var(--text-disabled)',
                  textTransform: 'uppercase',
                  letterSpacing: '0.12em',
                  marginBottom: '4px'
                }}>
                  Archive
                </div>
                <div style={{ fontSize: '12px', color: canArchive ? 'var(--error)' : 'var(--warning)', lineHeight: '1.5' }}>
                  {canArchive ? 'Ready to archive' : `${pendingTaskCount} pending task${pendingTaskCount === 1 ? '' : 's'} remaining`}
                </div>
              </div>
              <button
                onClick={() => canArchive && setShowArchiveConfirm(true)}
                disabled={!canArchive || isSaving}
                className="btn-ghost"
                title={canArchive ? 'Archive project' : 'Complete pending tasks before archiving'}
                style={{
                  color: canArchive ? 'var(--error)' : 'var(--text-disabled)',
                  borderColor: canArchive ? 'var(--error)' : 'var(--border)',
                  background: canArchive ? 'var(--error-muted)' : 'transparent',
                  fontSize: '12px',
                  padding: '8px 14px',
                  justifyContent: 'center'
                }}
              >
                {isSaving ? <Loader2 className="animate-spin" size={14} /> : <Archive size={14} />}
                Archive Project
              </button>
            </div>
          )}
        </div>
      </div>

      {showArchiveConfirm && (
        <div
          className="modal-overlay"
          style={{ zIndex: 130, background: 'rgba(0, 0, 0, 0.72)', backdropFilter: 'blur(12px)' }}
          onClick={(e) => {
            e.stopPropagation()
            if (e.target.classList.contains('modal-overlay')) setShowArchiveConfirm(false)
          }}
          onKeyDown={(e) => {
            e.stopPropagation()
            if (e.key === 'Escape') setShowArchiveConfirm(false)
          }}
        >
          <div className="modal-content" style={{
            maxWidth: '420px',
            background: 'var(--surface)',
            borderRadius: 'var(--radius-xl)',
            overflow: 'hidden',
            borderRight: '1px solid var(--border-strong)',
            borderBottom: '1px solid var(--border-strong)',
            borderLeft: '1px solid var(--border-strong)',
            borderTop: '2px solid var(--error)'
          }}>
            <div style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '16px 24px',
              background: 'var(--surface-alt)',
              borderBottom: '1px solid var(--border)'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <div style={{
                  width: '28px',
                  height: '28px',
                  borderRadius: 'var(--radius-sm)',
                  background: 'var(--error-muted)',
                  border: '1px solid var(--error)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0
                }}>
                  <Archive size={14} color="var(--error)" />
                </div>
                <div>
                  <div style={{ fontSize: '13px', fontWeight: '700', letterSpacing: '-0.01em', color: 'var(--text)' }}>
                    Archive Project
                  </div>
                  <div style={{ fontSize: '10px', color: 'var(--text-disabled)', fontFamily: 'var(--font-mono)', letterSpacing: '0.08em', marginTop: '2px' }}>
                    MOVE TO ARCHIVE
                  </div>
                </div>
              </div>
              <button
                onClick={() => setShowArchiveConfirm(false)}
                className="btn-ghost"
                style={{ padding: '6px', borderRadius: 'var(--radius-sm)', flexShrink: 0 }}
                disabled={isSaving}
              >
                <X size={16} />
              </button>
            </div>

            <div style={{ padding: '20px 24px' }}>
              <p style={{ fontSize: '13px', color: 'var(--text-secondary)', lineHeight: '1.6' }}>
                Archive <span style={{ fontWeight: '700', color: 'var(--text)' }}>"{edited.name}"</span>?
              </p>
              <p style={{ fontSize: '11px', color: 'var(--text-disabled)', marginTop: '8px', lineHeight: '1.5' }}>
                This removes the project from the active board. You can restore it from Archived Projects.
              </p>
            </div>

            <div style={{
              padding: '16px 24px',
              borderTop: '1px solid var(--border)',
              display: 'flex',
              justifyContent: 'flex-end',
              gap: '8px',
              background: 'var(--surface-alt)'
            }}>
              <button
                onClick={() => setShowArchiveConfirm(false)}
                className="btn-ghost"
                style={{ fontSize: '12px', padding: '8px 16px' }}
                disabled={isSaving}
              >
                Cancel
              </button>
              <button
                onClick={archiveProject}
                disabled={isSaving}
                style={{
                  fontSize: '12px',
                  padding: '8px 16px',
                  background: 'var(--error)',
                  color: 'white',
                  border: 'none',
                  borderRadius: 'var(--radius-md)',
                  display: 'inline-flex',
                  alignItems: 'center',
                  gap: '8px',
                  cursor: isSaving ? 'not-allowed' : 'pointer',
                  opacity: isSaving ? 0.7 : 1
                }}
              >
                {isSaving ? <Loader2 size={14} className="animate-spin" /> : <Archive size={14} />}
                Archive
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
