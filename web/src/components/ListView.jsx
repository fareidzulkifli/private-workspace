'use client';

import { useState, useRef } from 'react'
import {
  ChevronRight,
  ChevronDown,
  Calendar,
  CheckCircle2,
  Circle,
  MoreVertical,
  Maximize2,
  FileText,
  Plus,
  X,
  Loader2,
  ArrowRight,
  History,
  Info,
  Trash2
} from 'lucide-react'
import ProjectModal from './ProjectModal'

export default function ListView({ projects, tasks, onTaskClick, onTaskPatch, onViewCompleted, onTaskCreated, onTaskDeleted, onProjectDeleted, onProjectUpdated, onProjectArchived }) {
  const [expandedTaskId, setExpandedTaskId] = useState(null)

  const headers = [
    { label: 'Status', align: 'center' },
    { label: 'Task', align: 'left' },
    { label: 'Due Date', align: 'left' },
    { label: 'Priority', align: 'left' },
    { label: 'Actions', align: 'right' }
  ]

  return (
    <div className="list-view task-list-view" style={{
      padding: '0 32px 32px 32px',
      height: 'calc(100vh - 64px)',
      overflowY: 'auto',
      display: 'flex',
      flexDirection: 'column',
    }}>
      {/* Global List Header */}
      <div className="list-view-header task-list-header" style={{
        display: 'grid',
        gridTemplateColumns: '48px minmax(300px, 2fr) 120px 140px 80px',
        padding: '0 16px 10px 16px',
        marginTop: '24px',
        borderBottom: '2px solid var(--border-strong)',
      }}>
        {headers.map((h, i) => (
          <div key={i} style={{
            fontSize: '10px',
            fontWeight: '700',
            textTransform: 'uppercase',
            letterSpacing: '0.1em',
            color: 'var(--text-disabled)',
            textAlign: h.align,
            display: 'flex',
            alignItems: 'center',
            justifyContent: h.align === 'center' ? 'center' : h.align === 'right' ? 'flex-end' : 'flex-start'
          }}>
            {h.label}
          </div>
        ))}
      </div>

      {projects.map(project => {
        const priorityScore = (t) => (t.urgent && t.important) ? 0 : t.urgent ? 1 : t.important ? 2 : 3
        const projectTasks = tasks.filter(t => t.project_id === project.id && t.status !== 'Done')
          .sort((a, b) => {
            const pDiff = priorityScore(a) - priorityScore(b)
            if (pDiff !== 0) return pDiff
            return new Date(a.created_at) - new Date(b.created_at)
          })

        return (
          <ProjectGroup
            key={project.id}
            project={project}
            tasks={projectTasks}
            onTaskClick={onTaskClick}
            onTaskPatch={onTaskPatch}
            onViewCompleted={() => onViewCompleted(project)}
            expandedTaskId={expandedTaskId}
            onToggleExpand={(taskId) => setExpandedTaskId(prev => prev === taskId ? null : taskId)}
            onTaskCreated={onTaskCreated}
            onTaskDeleted={onTaskDeleted}
            onProjectDeleted={onProjectDeleted}
            onProjectUpdated={onProjectUpdated}
            onProjectArchived={onProjectArchived}
          />
        )
      })}

      {projects.length === 0 && (
        <div style={{
          padding: '48px',
          marginTop: '24px',
          textAlign: 'center',
          border: '1px dashed var(--border-strong)',
          borderRadius: '12px',
          color: 'var(--text-disabled)',
          fontSize: '13px'
        }}>
          No projects found. Create a project to get started.
        </div>
      )}
    </div>
  )
}

function ProjectGroup({ project, tasks, onTaskClick, onTaskPatch, onViewCompleted, expandedTaskId, onToggleExpand, onTaskCreated, onTaskDeleted, onProjectDeleted, onProjectUpdated, onProjectArchived }) {
  const [isExpanded, setIsExpanded] = useState(true)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showMenu, setShowMenu] = useState(false)
  const [newTaskSummary, setNewTaskSummary] = useState('')
  const [newTaskUrgent, setNewTaskUrgent] = useState(false)
  const [newTaskImportant, setNewTaskImportant] = useState(false)
  const [loading, setLoading] = useState(false)
  const [detailProject, setDetailProject] = useState(null)

  const handleOpenCreate = (e) => {
    e.stopPropagation()
    setNewTaskSummary('')
    setNewTaskUrgent(false)
    setNewTaskImportant(false)
    setShowCreateModal(true)
  }

  const handleEditProject = () => {
    setDetailProject(project)
    setShowMenu(false)
  }

  const handleDeleteProject = async () => {
    if (!confirm('Are you sure you want to delete this project? All tasks will be deleted.')) return
    try {
      await fetch(`/api/projects/${project.id}`, { method: 'DELETE' })
      onProjectDeleted(project.id)
      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Error deleting project: ' + err.message)
    } finally {
      setShowMenu(false)
    }
  }

  const handleCreateTask = async () => {
    if (!newTaskSummary.trim()) return
    try {
      setLoading(true)
      const res = await fetch('/api/tasks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          summary: newTaskSummary.trim(),
          project_id: project.id,
          order_index: tasks.length,
          urgent: newTaskUrgent,
          important: newTaskImportant,
        }),
      })
      const newTask = await res.json()
      if (newTask.error) throw new Error(newTask.error)
      onTaskCreated(newTask)
      setShowCreateModal(false)
      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Error: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const priorityBorderColor =
    newTaskUrgent && newTaskImportant ? 'var(--error)' :
    newTaskUrgent ? 'var(--warning)' :
    newTaskImportant ? 'var(--accent)' :
    'var(--border-strong)'

  return (
    <>
      {detailProject && (
        <ProjectModal
          project={detailProject}
          onClose={() => setDetailProject(null)}
          onProjectUpdate={(updated) => {
            setDetailProject(updated)
            onProjectUpdated?.(updated)
          }}
          onProjectArchived={(archivedProject) => {
            setDetailProject(null)
            onProjectArchived?.(archivedProject)
          }}
          pendingTaskCount={tasks.length}
        />
      )}

      {/* Flat section — no card wrapper */}
      <div className="project-section task-project-section">
        {/* Section Header */}
        <div
          className="project-section-header task-project-header"
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '20px 16px 10px 16px',
            borderBottom: '1px solid var(--border)',
          }}
        >
          {/* Left: collapse toggle + name + details btn + count */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <button
              onClick={() => setIsExpanded(!isExpanded)}
              style={{
                background: 'transparent',
                border: 'none',
                padding: '0',
                color: 'var(--text-muted)',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                flexShrink: 0,
              }}
            >
              {isExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            </button>
            <span
              onClick={() => {
                if (window.getSelection().toString().length === 0) {
                  setIsExpanded(!isExpanded)
                }
              }}
              style={{
                fontSize: '13px',
                fontWeight: '700',
                color: 'var(--text)',
                letterSpacing: '-0.01em',
                cursor: 'pointer',
              }}
            >
              {project.name}
            </span>
            <button
              onClick={(e) => { e.stopPropagation(); handleEditProject(); }}
              title="Project Details"
              style={{
                background: 'transparent',
                border: 'none',
                padding: '2px',
                color: 'var(--text-disabled)',
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                flexShrink: 0,
                borderRadius: '4px',
              }}
            >
              <Info size={13} />
            </button>
            <span className="project-task-count" style={{
              fontSize: '10px',
              fontWeight: '600',
              color: 'var(--text-disabled)',
              background: 'var(--surface-alt)',
              padding: '2px 8px',
              borderRadius: '999px',
              border: '1px solid var(--border-strong)',
            }}>
              {tasks.length}
            </span>
          </div>

          {/* Right: action buttons */}
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
            {/* Desktop actions */}
            <div className="desktop-project-actions" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <button
                onClick={(e) => { e.stopPropagation(); onViewCompleted(); }}
                className="lv-pill-btn"
                title="View Completed Tasks"
              >
                <History size={11} />
                <span>History</span>
              </button>
              <button
                onClick={(e) => { e.stopPropagation(); handleDeleteProject(); }}
                className="lv-pill-btn"
                title="Delete Project"
                style={{ padding: '4px 8px', color: 'var(--error)' }}
              >
                <Trash2 size={12} />
              </button>
              <div style={{ width: '1px', height: '14px', background: 'var(--border-strong)', margin: '0 4px' }} />
              <button
                onClick={handleOpenCreate}
                className="lv-pill-btn lv-pill-btn-primary"
              >
                <Plus size={11} />
                <span>Add Task</span>
              </button>
            </div>

            {/* Mobile add button */}
            <button
              onClick={handleOpenCreate}
              className="mobile-add-btn btn-ghost"
              style={{
                display: 'none',
                padding: '5px',
                borderRadius: '6px',
                border: '1px solid var(--accent)',
                color: 'var(--accent)'
              }}
            >
              <Plus size={14} />
            </button>

            {/* Mobile menu */}
            <div className="mobile-project-menu" style={{ position: 'relative' }}>
              <button
                onClick={(e) => { e.stopPropagation(); setShowMenu(!showMenu); }}
                className="btn-ghost"
                style={{ padding: '4px', height: 'auto', color: 'var(--text-muted)' }}
              >
                <MoreVertical size={16} />
              </button>

              {showMenu && (
                <>
                  <div
                    style={{ position: 'fixed', inset: 0, zIndex: 90 }}
                    onClick={(e) => { e.stopPropagation(); setShowMenu(false); }}
                  />
                  <div style={{
                    position: 'absolute',
                    top: '100%',
                    right: 0,
                    marginTop: '8px',
                    width: '180px',
                    background: 'var(--surface)',
                    border: '1px solid var(--border)',
                    borderRadius: '8px',
                    boxShadow: 'var(--shadow-lg)',
                    zIndex: 100,
                    display: 'flex',
                    flexDirection: 'column',
                    padding: '4px'
                  }}>
                    <button
                      onClick={(e) => { e.stopPropagation(); onViewCompleted(); setShowMenu(false); }}
                      className="btn-ghost"
                      style={{ justifyContent: 'flex-start', padding: '8px 12px', fontSize: '12px' }}
                    >
                      <History size={14} /> History
                    </button>
                    <div style={{ height: '1px', background: 'var(--border)', margin: '4px 0' }} />
                    <button
                      onClick={(e) => { e.stopPropagation(); handleEditProject(); }}
                      className="btn-ghost"
                      style={{ justifyContent: 'flex-start', padding: '8px 12px', fontSize: '12px' }}
                    >
                      <Info size={14} /> Details
                    </button>
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDeleteProject(); }}
                      className="btn-ghost"
                      style={{ justifyContent: 'flex-start', padding: '8px 12px', fontSize: '12px', color: 'var(--error)' }}
                    >
                      <Trash2 size={14} /> Delete
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>
        </div>

        {/* Task Rows */}
        {isExpanded && (
          <div>
            {tasks.map(task => (
              <TaskRow
                key={task.id}
                task={task}
                onTaskClick={onTaskClick}
                onTaskPatch={onTaskPatch}
                isExpanded={expandedTaskId === task.id}
                onToggleExpand={() => onToggleExpand(task.id)}
                onTaskDeleted={onTaskDeleted}
              />
            ))}
            {tasks.length === 0 && (
              <div className="task-empty-row" style={{
                padding: '14px 16px 14px 64px',
                fontSize: '11px',
                color: 'var(--text-disabled)',
                fontStyle: 'italic',
                borderBottom: '1px solid var(--border)'
              }}>
                No active tasks
              </div>
            )}
          </div>
        )}
      </div>

      {/* New Task Creation Modal */}
      {showCreateModal && (
        <div
          className="modal-overlay"
          onClick={(e) => e.target.classList.contains('modal-overlay') && setShowCreateModal(false)}
          onKeyDown={(e) => e.key === 'Escape' && setShowCreateModal(false)}
        >
          <div className="modal-content" style={{
            background: 'var(--surface)',
            borderRadius: 'var(--radius-xl)',
            boxShadow: 'var(--shadow-xl)',
            width: '100%',
            maxWidth: '520px',
            overflow: 'hidden',
            borderRight: '1px solid var(--border-strong)',
            borderBottom: '1px solid var(--border-strong)',
            borderLeft: '1px solid var(--border-strong)',
            borderTop: `2px solid ${priorityBorderColor}`,
            transition: 'border-top-color 0.2s ease'
          }}>

            {/* Header */}
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '16px 24px',
              background: 'var(--surface-alt)',
              borderBottom: '1px solid var(--border)'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <div style={{
                  width: '28px', height: '28px', borderRadius: 'var(--radius-sm)',
                  background: 'var(--accent-muted)', border: '1px solid var(--accent-subtle)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0
                }}>
                  <Plus size={14} color="var(--accent)" />
                </div>
                <div>
                  <div style={{ fontSize: '13px', fontWeight: '700', letterSpacing: '-0.01em', color: 'var(--text)' }}>New task</div>
                  <div style={{ fontSize: '10px', color: 'var(--text-disabled)', fontFamily: 'var(--font-mono)', letterSpacing: '0.08em', marginTop: '2px' }}>
                    {project.name}
                  </div>
                </div>
              </div>
              <button
                onClick={() => setShowCreateModal(false)}
                className="btn-ghost"
                style={{ padding: '6px', borderRadius: 'var(--radius-sm)', flexShrink: 0 }}
              >
                <X size={16} />
              </button>
            </div>

            {/* Body */}
            <div style={{ padding: '28px 24px', display: 'flex', flexDirection: 'column', gap: '24px' }}>

              {/* Summary Input */}
              <div>
                <label style={{
                  display: 'block', fontSize: '10px', fontWeight: '800',
                  color: 'var(--text-disabled)', textTransform: 'uppercase',
                  letterSpacing: '0.15em', marginBottom: '10px'
                }}>
                  Task Summary
                </label>
                <textarea
                  autoFocus
                  value={newTaskSummary}
                  onChange={e => {
                    let val = e.target.value;
                    if (val.includes(' ')) {
                      val = val.charAt(0).toUpperCase() + val.slice(1);
                    }
                    setNewTaskSummary(val);
                  }}
                  onKeyDown={e => {
                    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleCreateTask(); }
                  }}
                  placeholder="Describe the task objective..."
                  rows={3}
                  style={{
                    resize: 'none',
                    fontSize: '15px',
                    fontWeight: '500',
                    lineHeight: '1.6',
                    background: 'var(--background)',
                    border: '1px solid var(--border-strong)',
                    borderRadius: 'var(--radius-md)',
                    padding: '14px 16px',
                    color: 'var(--text)',
                    width: '100%'
                  }}
                />
              </div>

              {/* Priority Flags */}
              <div>
                <label style={{
                  display: 'block', fontSize: '10px', fontWeight: '800',
                  color: 'var(--text-disabled)', textTransform: 'uppercase',
                  letterSpacing: '0.15em', marginBottom: '10px'
                }}>
                  Priority Flags
                </label>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px' }}>

                  {/* Urgent */}
                  <button
                    onClick={() => setNewTaskUrgent(v => !v)}
                    style={{
                      display: 'flex', flexDirection: 'column', alignItems: 'flex-start',
                      gap: '8px', padding: '14px', borderRadius: 'var(--radius-md)',
                      background: newTaskUrgent ? 'var(--warning-muted)' : 'var(--background)',
                      border: `1px solid ${newTaskUrgent ? 'var(--warning)' : 'var(--border-strong)'}`,
                      cursor: 'pointer', transition: 'all 0.15s ease',
                      color: newTaskUrgent ? 'var(--warning)' : 'var(--text-disabled)',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%' }}>
                      <div style={{
                        width: '22px', height: '22px', borderRadius: 'var(--radius-sm)',
                        background: newTaskUrgent ? 'var(--warning-muted)' : 'var(--surface-alt)',
                        border: `1px solid ${newTaskUrgent ? 'var(--warning)' : 'var(--border-strong)'}`,
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: '10px', fontWeight: '900', fontFamily: 'var(--font-mono)',
                        transition: 'all 0.15s',
                        color: newTaskUrgent ? 'var(--warning)' : 'var(--text-disabled)'
                      }}>U</div>
                      <div style={{
                        width: '6px', height: '6px', borderRadius: '50%',
                        background: newTaskUrgent ? 'var(--warning)' : 'var(--border-strong)',
                        transition: 'background 0.15s', flexShrink: 0
                      }} />
                    </div>
                    <div style={{ textAlign: 'left' }}>
                      <div style={{ fontSize: '11px', fontWeight: '800', letterSpacing: '0.06em' }}>URGENT</div>
                      <div style={{ fontSize: '10px', color: 'var(--text-disabled)', marginTop: '2px', fontWeight: '400' }}>Time-sensitive</div>
                    </div>
                  </button>

                  {/* Important */}
                  <button
                    onClick={() => setNewTaskImportant(v => !v)}
                    style={{
                      display: 'flex', flexDirection: 'column', alignItems: 'flex-start',
                      gap: '8px', padding: '14px', borderRadius: 'var(--radius-md)',
                      background: newTaskImportant ? 'var(--accent-muted)' : 'var(--background)',
                      border: `1px solid ${newTaskImportant ? 'var(--accent)' : 'var(--border-strong)'}`,
                      cursor: 'pointer', transition: 'all 0.15s ease',
                      color: newTaskImportant ? 'var(--accent)' : 'var(--text-disabled)',
                    }}
                  >
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%' }}>
                      <div style={{
                        width: '22px', height: '22px', borderRadius: 'var(--radius-sm)',
                        background: newTaskImportant ? 'var(--accent-subtle)' : 'var(--surface-alt)',
                        border: `1px solid ${newTaskImportant ? 'var(--accent)' : 'var(--border-strong)'}`,
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: '10px', fontWeight: '900', fontFamily: 'var(--font-mono)',
                        transition: 'all 0.15s',
                        color: newTaskImportant ? 'var(--accent)' : 'var(--text-disabled)'
                      }}>I</div>
                      <div style={{
                        width: '6px', height: '6px', borderRadius: '50%',
                        background: newTaskImportant ? 'var(--accent)' : 'var(--border-strong)',
                        transition: 'background 0.15s', flexShrink: 0
                      }} />
                    </div>
                    <div style={{ textAlign: 'left' }}>
                      <div style={{ fontSize: '11px', fontWeight: '800', letterSpacing: '0.06em' }}>IMPORTANT</div>
                      <div style={{ fontSize: '10px', color: 'var(--text-disabled)', marginTop: '2px', fontWeight: '400' }}>High impact</div>
                    </div>
                  </button>
                </div>

                {/* Quadrant label */}
                {(newTaskUrgent || newTaskImportant) && (
                  <div style={{
                    marginTop: '10px', padding: '7px 12px', borderRadius: 'var(--radius-sm)',
                    background: newTaskUrgent && newTaskImportant ? 'var(--error-muted)' : newTaskUrgent ? 'var(--warning-muted)' : 'var(--accent-subtle)',
                    border: `1px solid ${newTaskUrgent && newTaskImportant ? 'var(--error)' : newTaskUrgent ? 'var(--warning)' : 'var(--accent)'}`,
                    display: 'flex', alignItems: 'center', gap: '8px'
                  }}>
                    <div style={{
                      width: '4px', height: '4px', borderRadius: '50%', flexShrink: 0,
                      background: newTaskUrgent && newTaskImportant ? 'var(--error)' : newTaskUrgent ? 'var(--warning)' : 'var(--accent)'
                    }} />
                    <span style={{
                      fontSize: '10px', fontWeight: '800', fontFamily: 'var(--font-mono)', letterSpacing: '0.06em',
                      color: newTaskUrgent && newTaskImportant ? 'var(--error)' : newTaskUrgent ? 'var(--warning)' : 'var(--accent)'
                    }}>
                      {newTaskUrgent && newTaskImportant ? 'Do first'
                        : newTaskUrgent ? 'Fast track'
                        : 'High impact'}
                    </span>
                  </div>
                )}
              </div>
            </div>

            {/* Footer */}
            <div style={{
              padding: '16px 24px', borderTop: '1px solid var(--border)',
              display: 'flex', justifyContent: 'flex-end', gap: '8px',
              background: 'var(--surface-alt)'
            }}>
              <button
                onClick={() => setShowCreateModal(false)}
                className="btn-ghost"
                style={{ fontSize: '12px', padding: '8px 16px' }}
              >
                Cancel
              </button>
              <button
                onClick={handleCreateTask}
                disabled={!newTaskSummary.trim() || loading}
                className="btn-primary"
                style={{ fontSize: '12px', padding: '8px 20px', gap: '8px' }}
              >
                {loading ? <Loader2 size={14} className="animate-spin" /> : <ArrowRight size={14} />}
                Create Task
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

function TaskRow({ task, onTaskClick, onTaskPatch, isExpanded, onToggleExpand, onTaskDeleted }) {
  const isDone = task.status === 'Done'
  const priorityTone = task.urgent && task.important ? 'critical' : task.urgent ? 'urgent' : task.important ? 'important' : 'normal'
  const [swipeOffset, setSwipeOffset] = useState(0)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const touchStartRef = useRef(null)
  const isHorizontalRef = useRef(false)
  const swipingRef = useRef(false)

  const patchTask = async (updates) => {
    try {
      const res = await fetch(`/api/tasks/${task.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updates),
      })
      const serverTask = await res.json()
      if (!serverTask.error) {
        onTaskPatch(task.id, serverTask)
      }
    } catch (err) {
      console.error('Failed to patch task:', err)
      alert('Failed to update task')
    }
  }

  const handleToggleDone = (e) => {
    e.stopPropagation()
    const newStatus = isDone ? 'In Progress' : 'Done'
    onTaskPatch(task.id, { status: newStatus })
    patchTask({ status: newStatus })
  }

  const handleToggleFlag = (e, flag) => {
    e.stopPropagation()
    const newVal = !task[flag]
    onTaskPatch(task.id, { [flag]: newVal })
    patchTask({ [flag]: newVal })
  }

  const handleTouchStart = (e) => {
    if (isExpanded) return
    touchStartRef.current = { x: e.touches[0].clientX, y: e.touches[0].clientY }
    isHorizontalRef.current = false
    swipingRef.current = false
  }

  const handleTouchMove = (e) => {
    if (!touchStartRef.current || isExpanded) return
    const dx = e.touches[0].clientX - touchStartRef.current.x
    const dy = e.touches[0].clientY - touchStartRef.current.y
    if (!swipingRef.current && (Math.abs(dx) > 8 || Math.abs(dy) > 8)) {
      isHorizontalRef.current = Math.abs(dx) > Math.abs(dy)
      swipingRef.current = true
    }
    if (!isHorizontalRef.current || dx > 0) return
    setSwipeOffset(Math.max(-80, dx))
  }

  const handleTouchEnd = () => {
    if (!touchStartRef.current) return
    if (swipeOffset < -50) {
      setSwipeOffset(0)
      setTimeout(() => setShowDeleteConfirm(true), 150)
    } else {
      setSwipeOffset(0)
    }
    touchStartRef.current = null
    isHorizontalRef.current = false
    swipingRef.current = false
  }

  const handleDeleteTask = async () => {
    try {
      setDeleting(true)
      await fetch(`/api/tasks/${task.id}`, { method: 'DELETE' })
      onTaskDeleted(task.id)
      setShowDeleteConfirm(false)
      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Failed to delete task')
      setDeleting(false)
    }
  }

  return (
    <>
      <div
        className={`task-row-container task-row-${priorityTone} ${isExpanded ? 'is-expanded' : ''} ${isDone ? 'is-done' : ''}`}
        style={{
          position: 'relative',
          borderBottom: '1px solid var(--border)',
          background: isExpanded ? 'var(--surface-alt)' : 'transparent',
          transition: 'background 0.15s ease',
        }}
        onTouchStart={handleTouchStart}
        onTouchMove={handleTouchMove}
        onTouchEnd={handleTouchEnd}
      >
        {/* Delete strip — revealed on left-swipe (mobile) */}
        <div
          className="task-delete-strip"
          style={{
            position: 'absolute',
            top: 0, right: 0, bottom: 0,
            width: '80px',
            background: 'var(--error)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexDirection: 'column',
            gap: '4px',
            opacity: Math.abs(swipeOffset) > 10 ? Math.abs(swipeOffset) / 80 : 0,
            transition: swipeOffset === 0 ? 'opacity 0.25s ease' : 'none',
            zIndex: 0,
            userSelect: 'none',
            pointerEvents: 'none'
          }}
        >
          <Trash2 size={20} color="white" />
          <span style={{ fontSize: '9px', fontWeight: '800', color: 'white', letterSpacing: '0.1em' }}>DELETE</span>
        </div>

        {/* Sliding content wrapper */}
        <div
          className="task-row-sliding-wrapper"
          style={{
            transform: `translateX(${swipeOffset}px)`,
            transition: swipeOffset === 0 ? 'transform 0.3s cubic-bezier(0.4, 0, 0.2, 1)' : 'none',
            position: 'relative',
            zIndex: 1,
          }}
        >
          {/* Main Row */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: '48px minmax(300px, 2fr) 120px 140px 80px',
              alignItems: 'center',
              padding: '0 16px 0 44px',
              minHeight: '44px',
              cursor: 'pointer',
            }}
            className={`task-row-grid ${isExpanded ? 'is-expanded' : ''}`}
            onClick={onToggleExpand}
          >
            {/* Status */}
            <div style={{ display: 'flex', justifyContent: 'center' }}>
              <button
                onClick={handleToggleDone}
                style={{
                  background: 'transparent',
                  border: 'none',
                  padding: '4px',
                  color: isDone ? 'var(--success)' : 'var(--text-disabled)',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center'
                }}
              >
                {isDone ? <CheckCircle2 size={16} fill="var(--success-muted)" /> : <Circle size={16} />}
              </button>
            </div>

            {/* Task Title */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', paddingRight: '16px' }}>
              <span style={{
                fontSize: '13px',
                fontWeight: '400',
                color: isDone ? 'var(--text-disabled)' : 'var(--text)',
                textDecoration: isDone ? 'line-through' : 'none',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis'
              }}>
                {task.summary}
              </span>
              {task.notes_markdown && <FileText size={11} color="var(--text-disabled)" />}
            </div>

            {/* Due Date */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: '6px', fontSize: '12px', padding: '0 0 0 0', color: task.due_date ? 'var(--text-secondary)' : 'var(--text-disabled)' }}>
              {task.due_date ? (
                <>
                  <Calendar size={12} />
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '11px' }}>{task.due_date}</span>
                </>
              ) : (
                <span style={{ opacity: 0.3 }}>—</span>
              )}
            </div>

            {/* Priority — pill badges */}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '4px', padding: '0 30px 0 0'}}>
              <button
                onClick={(e) => handleToggleFlag(e, 'urgent')}
                title="Toggle Urgency"
                style={{
                  padding: '2px 9px',
                  fontSize: '10px',
                  fontWeight: '700',
                  letterSpacing: '0.04em',
                  borderRadius: '999px',
                  background: task.urgent ? 'var(--warning-muted)' : 'transparent',
                  color: task.urgent ? 'var(--warning)' : 'var(--text-disabled)',
                  border: `1px solid ${task.urgent ? 'var(--warning)' : 'var(--border-strong)'}`,
                  transition: 'all 0.12s',
                  cursor: 'pointer'
                }}
              >
                U
              </button>
              <button
                onClick={(e) => handleToggleFlag(e, 'important')}
                title="Toggle Importance"
                style={{
                  padding: '2px 9px',
                  fontSize: '10px',
                  fontWeight: '700',
                  letterSpacing: '0.04em',
                  borderRadius: '999px',
                  background: task.important ? 'var(--accent-muted)' : 'transparent',
                  color: task.important ? 'var(--accent)' : 'var(--text-disabled)',
                  border: `1px solid ${task.important ? 'var(--accent)' : 'var(--border-strong)'}`,
                  transition: 'all 0.12s',
                  cursor: 'pointer'
                }}
              >
                I
              </button>
            </div>



            {/* Actions */}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '4px' }}>
              <button
                onClick={(e) => { e.stopPropagation(); onTaskClick(task); }}
                className="btn-ghost"
                style={{ padding: '6px', height: 'auto' }}
                title="Open Details"
              >
                <Maximize2 size={14} />
              </button>
              <button
                onClick={(e) => { e.stopPropagation(); setShowDeleteConfirm(true); }}
                className="lv-row-delete-btn"
                title="Delete Task"
              >
                <Trash2 size={14} />
              </button>
            </div>
          </div>

          {/* Expanded Details Row */}
          {isExpanded && (
            <div style={{
              padding: '12px 16px 16px 64px',
              display: 'flex',
              flexDirection: 'column',
              gap: '12px',
              cursor: 'default',
              borderTop: '1px solid var(--border)',
            }}>
              {task.notes_markdown && (
                <div style={{
                  fontSize: '12px',
                  color: 'var(--text-secondary)',
                  lineHeight: '1.6',
                  background: 'var(--background)',
                  padding: '12px',
                  borderRadius: '6px',
                  border: '1px solid var(--border)',
                  maxWidth: '80%'
                }}>
                  <div style={{ fontSize: '10px', fontWeight: '700', color: 'var(--text-disabled)', marginBottom: '4px', textTransform: 'uppercase' }}>Notes Snippet</div>
                  {task.notes_markdown.slice(0, 300) + (task.notes_markdown.length > 300 ? '...' : '')}
                </div>
              )}
              <div style={{ display: 'flex', gap: '8px' }}>
                <button
                  onClick={() => onTaskClick(task)}
                  className="btn-primary"
                  style={{ fontSize: '11px', padding: '6px 12px', height: '28px' }}
                >
                  Full Details
                </button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Delete Confirmation Modal */}
      {showDeleteConfirm && (
        <div
          className="modal-overlay"
          onClick={(e) => e.target.classList.contains('modal-overlay') && setShowDeleteConfirm(false)}
          onKeyDown={(e) => e.key === 'Escape' && setShowDeleteConfirm(false)}
        >
          <div className="modal-content" style={{
            maxWidth: '380px',
            background: 'var(--surface)',
            borderRadius: 'var(--radius-xl)',
            overflow: 'hidden',
            borderRight: '1px solid var(--border-strong)',
            borderBottom: '1px solid var(--border-strong)',
            borderLeft: '1px solid var(--border-strong)',
            borderTop: '2px solid var(--error)'
          }}>
            <div style={{
              display: 'flex', alignItems: 'center', justifyContent: 'space-between',
              padding: '16px 24px',
              background: 'var(--surface-alt)',
              borderBottom: '1px solid var(--border)'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <div style={{
                  width: '28px', height: '28px', borderRadius: 'var(--radius-sm)',
                  background: 'var(--error-muted)', border: '1px solid var(--error)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0
                }}>
                  <Trash2 size={14} color="var(--error)" />
                </div>
                <div>
                  <div style={{ fontSize: '13px', fontWeight: '700', letterSpacing: '-0.01em', color: 'var(--text)' }}>Delete Task</div>
                  <div style={{ fontSize: '10px', color: 'var(--text-disabled)', fontFamily: 'var(--font-mono)', letterSpacing: '0.08em', marginTop: '2px' }}>
                    Permanent action
                  </div>
                </div>
              </div>
              <button onClick={() => setShowDeleteConfirm(false)} className="btn-ghost" style={{ padding: '6px', borderRadius: 'var(--radius-sm)', flexShrink: 0 }}>
                <X size={16} />
              </button>
            </div>

            <div style={{ padding: '20px 24px' }}>
              <p style={{ fontSize: '13px', color: 'var(--text-secondary)', lineHeight: '1.6' }}>
                Are you sure you want to permanently delete{' '}
                <span style={{ fontWeight: '700', color: 'var(--text)' }}>"{task.summary}"</span>?
              </p>
              <p style={{ fontSize: '11px', color: 'var(--text-disabled)', marginTop: '8px' }}>
                This action cannot be undone.
              </p>
            </div>

            <div style={{
              padding: '16px 24px', borderTop: '1px solid var(--border)',
              display: 'flex', justifyContent: 'flex-end', gap: '8px',
              background: 'var(--surface-alt)'
            }}>
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="btn-ghost"
                style={{ fontSize: '12px', padding: '8px 16px' }}
                disabled={deleting}
              >
                Cancel
              </button>
              <button
                onClick={handleDeleteTask}
                disabled={deleting}
                style={{
                  fontSize: '12px', padding: '8px 16px',
                  background: 'var(--error)', color: 'white',
                  border: 'none', borderRadius: 'var(--radius-md)',
                  display: 'inline-flex', alignItems: 'center', gap: '8px',
                  cursor: deleting ? 'not-allowed' : 'pointer',
                  opacity: deleting ? 0.7 : 1
                }}
              >
                {deleting ? <Loader2 size={14} className="animate-spin" /> : <Trash2 size={14} />}
                Delete
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
