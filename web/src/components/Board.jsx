'use client';

import { useState, useEffect } from 'react'
import TaskModal from './TaskModal'
import ListView from './ListView'
import {
  Plus,
  Search,
  Loader2,
  Check,
  Database,
  Activity,
  CalendarClock,
  Layers,
  History,
  RefreshCcw,
  PanelLeft,
  Maximize2,
  Archive,
  ArchiveRestore
} from 'lucide-react'

// Priority score: urgent+important(0) > urgent(1) > important(2) > none(3)
const priorityScore = (t) => (t.urgent && t.important) ? 0 : t.urgent ? 1 : t.important ? 2 : 3

const projectOrderValue = (project) => {
  const value = Number(project?.order_index)
  return Number.isFinite(value) ? value : 0
}

// Done tasks sink to the bottom; within each group sort by priority, then
// order_index (preserves drag-and-drop reordering), then created_at as tiebreaker
const sortTasks = (taskList) =>
  [...taskList].sort((a, b) => {
    const aDone = a.status === 'Done' ? 1 : 0
    const bDone = b.status === 'Done' ? 1 : 0
    if (aDone !== bDone) return aDone - bDone
    const pDiff = priorityScore(a) - priorityScore(b)
    if (pDiff !== 0) return pDiff
    if (a.order_index !== b.order_index) return a.order_index - b.order_index
    return new Date(a.created_at) - new Date(b.created_at)
  })

const padDatePart = (value) => String(value).padStart(2, '0')

const formatCompletedAt = (value) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Date unknown'
  return `${date.getFullYear()}-${padDatePart(date.getMonth() + 1)}-${padDatePart(date.getDate())} ${padDatePart(date.getHours())}:${padDatePart(date.getMinutes())}`
}

const toDateTimeLocalValue = (value) => {
  const date = new Date(value || Date.now())
  if (Number.isNaN(date.getTime())) return ''
  return `${date.getFullYear()}-${padDatePart(date.getMonth() + 1)}-${padDatePart(date.getDate())}T${padDatePart(date.getHours())}:${padDatePart(date.getMinutes())}`
}

const fromDateTimeLocalValue = (value) => {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date.toISOString()
}

export default function Board({ orgId }) {
  const [org, setOrg] = useState(null)
  const [projects, setProjects] = useState([])
  const [tasks, setTasks] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [selectedTask, setSelectedTask] = useState(null)
  const [isTaskModalOpen, setIsTaskModalOpen] = useState(false)
  const [completedTasksProject, setCompletedTasksProject] = useState(null)
  const [completedAtEditor, setCompletedAtEditor] = useState(null)
  const [completedAtSaving, setCompletedAtSaving] = useState(false)
  const [isArchiveModalOpen, setIsArchiveModalOpen] = useState(false)
  const [archivedProjects, setArchivedProjects] = useState([])
  const [archivedLoading, setArchivedLoading] = useState(false)
  const [archivedError, setArchivedError] = useState(null)
  const [restoringProjectId, setRestoringProjectId] = useState(null)
  const [projectOrderSaving, setProjectOrderSaving] = useState(false)

  // New Project Modal State
  const [isProjectModalOpen, setIsProjectModalOpen] = useState(false)
  const [newProjectName, setNewProjectName] = useState('')

  const handleTaskPatch = (taskId, updates) => {
    let shouldDispatch = false
    setTasks(prev => {
      const current = prev.find(t => t.id === taskId)
      shouldDispatch = current != null && 'status' in updates && updates.status !== current.status
      return prev.map(t => t.id === taskId ? { ...t, ...updates } : t)
    })
    if (shouldDispatch) {
      window.dispatchEvent(new Event('taskUpdated'))
    }
  }

  const handleToggleSidebar = () => {
    window.dispatchEvent(new Event('toggle-sidebar'))
  }

  const fetchData = async (isInitial = false) => {
    try {
      if (isInitial) setLoading(true)
      const [orgRes, projectsRes, tasksRes] = await Promise.all([
        fetch(`/api/orgs/${orgId}`),
        fetch(`/api/projects?org_id=${orgId}`),
        fetch(`/api/tasks?org_id=${orgId}`),
      ])
      const [orgData, projectsData, tasksData] = await Promise.all([
        orgRes.json(),
        projectsRes.json(),
        tasksRes.json(),
      ])
      if (orgData.error) throw new Error(orgData.error)
      if (projectsData.error) throw new Error(projectsData.error)
      if (tasksData.error) throw new Error(tasksData.error)
      setOrg(orgData)
      setProjects(projectsData)
      setTasks(tasksData)
    } catch (err) {
      setError(err.message)
    } finally {
      if (isInitial) setLoading(false)
    }
  }

  useEffect(() => {
    if (orgId) fetchData(true)
  }, [orgId])

  const handleProjectsReordered = async (orderedProjects) => {
    if (projectOrderSaving) return

    const previousProjects = projects
    const previousById = new Map(previousProjects.map(project => [project.id, project]))
    const normalizedProjects = orderedProjects.map((project, index) => ({
      ...project,
      order_index: index,
    }))
    const changedProjects = normalizedProjects.filter(project => {
      const previousProject = previousById.get(project.id)
      return projectOrderValue(previousProject) !== project.order_index
    })

    setProjects(normalizedProjects)
    if (changedProjects.length === 0) return

    try {
      setProjectOrderSaving(true)
      await Promise.all(changedProjects.map(async project => {
        const res = await fetch(`/api/projects/${project.id}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ order_index: project.order_index }),
        })
        const updatedProject = await res.json()
        if (!res.ok || updatedProject.error) {
          throw new Error(updatedProject.error || `Failed to save ${project.name}`)
        }
      }))
      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Error saving project order: ' + err.message)
      setProjects(previousProjects)
      await fetchData(false)
    } finally {
      setProjectOrderSaving(false)
    }
  }

  const handleTaskCreated = (newTask) => {
    setTasks(prev => [...prev, newTask])
  }

  const handleTaskDeleted = (taskId) => {
    setTasks(prev => prev.filter(t => t.id !== taskId))
  }

  const handleProjectDeleted = (projectId) => {
    setProjects(prev => prev.filter(p => p.id !== projectId))
    setTasks(prev => prev.filter(t => t.project_id !== projectId))
  }

  const handleProjectUpdated = (updatedProject) => {
    setProjects(prev => prev.map(p => p.id === updatedProject.id ? updatedProject : p))
  }

  const handleProjectArchived = (archivedProject) => {
    setProjects(prev => prev.filter(p => p.id !== archivedProject.id))
    setTasks(prev => prev.filter(t => t.project_id !== archivedProject.id))
    setCompletedTasksProject(prev => prev?.id === archivedProject.id ? null : prev)
  }

  const fetchArchivedProjects = async () => {
    try {
      setArchivedLoading(true)
      setArchivedError(null)
      const res = await fetch(`/api/projects?org_id=${orgId}&archived=true`)
      const data = await res.json()
      if (data.error) throw new Error(data.error)
      setArchivedProjects(data)
    } catch (err) {
      setArchivedError(err.message)
    } finally {
      setArchivedLoading(false)
    }
  }

  const handleOpenArchivedProjects = () => {
    setIsArchiveModalOpen(true)
    fetchArchivedProjects()
  }

  const handleRestoreProject = async (projectId) => {
    try {
      setRestoringProjectId(projectId)
      const res = await fetch(`/api/projects/${projectId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ archived: false })
      })
      const restoredProject = await res.json()
      if (restoredProject.error) throw new Error(restoredProject.error)

      setArchivedProjects(prev => prev.filter(p => p.id !== restoredProject.id))
      setProjects(prev => {
        const next = [...prev.filter(p => p.id !== restoredProject.id), restoredProject]
        return next.sort((a, b) => (a.order_index || 0) - (b.order_index || 0))
      })

      const tasksRes = await fetch(`/api/tasks?project_id=${restoredProject.id}`)
      const projectTasks = await tasksRes.json()
      if (Array.isArray(projectTasks)) {
        setTasks(prev => [
          ...prev.filter(t => t.project_id !== restoredProject.id),
          ...projectTasks
        ])
      }

      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Error restoring project: ' + err.message)
    } finally {
      setRestoringProjectId(null)
    }
  }

  const openTaskModal = (task) => {
    setSelectedTask(task)
    setIsTaskModalOpen(true)
  }

  const handleCloseModal = () => {
    setIsTaskModalOpen(false)
    setSelectedTask(null)
  }

  const handleCreateProject = () => {
    setNewProjectName('')
    setIsProjectModalOpen(true)
  }

  const handleConfirmCreateProject = async () => {
    if (!newProjectName.trim()) return
    try {
      const res = await fetch('/api/projects', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: newProjectName.trim(),
          org_id: orgId,
          order_index: projects.length
        })
      })
      const newProject = await res.json()
      if (newProject.error) throw new Error(newProject.error)
      setProjects([...projects, newProject])
      window.dispatchEvent(new Event('taskUpdated'))
      setIsProjectModalOpen(false)
    } catch (err) {
      alert('Error: ' + err.message)
    }
  }

  const handleViewCompleted = (project) => {
    setCompletedTasksProject(project)
  }

  const openCompletedAtEditor = (task) => {
    setCompletedAtEditor({
      task,
      value: toDateTimeLocalValue(task.completed_at || task.created_at)
    })
  }

  const handleSaveCompletedAt = async () => {
    if (!completedAtEditor?.task || !completedAtEditor.value) return
    const completedAt = fromDateTimeLocalValue(completedAtEditor.value)
    if (!completedAt) {
      alert('Choose a valid completed date.')
      return
    }

    try {
      setCompletedAtSaving(true)
      const res = await fetch(`/api/tasks/${completedAtEditor.task.id}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ completed_at: completedAt })
      })
      const updatedTask = await res.json()
      if (updatedTask.error) throw new Error(updatedTask.error)

      setTasks(prev => sortTasks(prev.map(t => t.id === updatedTask.id ? updatedTask : t)))
      setSelectedTask(prev => prev?.id === updatedTask.id ? updatedTask : prev)
      setCompletedAtEditor(null)
      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Error updating completed date: ' + err.message)
    } finally {
      setCompletedAtSaving(false)
    }
  }

  const handleRestoreTask = async (taskId) => {
    try {
      const res = await fetch(`/api/tasks/${taskId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: 'In Progress' })
      })
      const updatedTask = await res.json()
      if (updatedTask.error) throw new Error(updatedTask.error)

      setTasks(prev => prev.map(t => t.id === taskId ? updatedTask : t))
      setCompletedAtEditor(prev => prev?.task?.id === taskId ? null : prev)
      window.dispatchEvent(new Event('taskUpdated'))
    } catch (err) {
      alert('Error restoring task: ' + err.message)
    }
  }

  if (loading) return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: '16px', background: 'var(--background)' }}>
      <Loader2 className="animate-spin" size={20} color="var(--accent)" />
      <span style={{ color: 'var(--text-muted)', fontWeight: '600', fontSize: '13px' }}>Loading workspace...</span>
    </div>
  )

  if (error) return <div style={{ padding: '40px', color: 'var(--error)', fontWeight: 600 }}>Unable to load workspace: {error}</div>

  return (
    <div className="board-view task-workspace">
      {/* Header */}
      <header className="board-header task-board-header">
        <div className="board-header-left task-board-header-left">
          <button
            className="mobile-menu-btn btn-ghost"
            onClick={handleToggleSidebar}
            style={{ display: 'none', padding: '8px' }}
          >
            <PanelLeft size={20} />
          </button>

          <div className="board-title-group task-board-title-group">
            <Database size={18} color="var(--accent)" className="desktop-only-icon" />
            <h1 className="board-title task-board-title">
              {org?.name}
            </h1>
          </div>

          <div className="header-divider task-header-divider"></div>

          <div className="board-stats task-board-stats">
            <div className="task-board-stat">
              <Layers size={14} />
              <span>{projects.length.toString().padStart(2, '0')} projects</span>
            </div>
            <div className="task-board-stat">
              <Activity size={14} />
              <span>{tasks.filter(t => t.status !== 'Done').length.toString().padStart(3, '0')} active tasks</span>
            </div>
          </div>
        </div>

        <div className="board-header-right task-board-toolbar">
          <div className="task-search-wrap">
            <Search size={14} className="task-search-icon" />
            <input
              type="text"
              placeholder="Search tasks"
              className="task-search-input"
            />
          </div>

          <button
            onClick={handleOpenArchivedProjects}
            className="btn-ghost board-archive-btn task-toolbar-btn"
          >
            <Archive size={14} />
            <span>Archived</span>
          </button>

          <button
            onClick={handleCreateProject}
            className="btn-primary task-toolbar-primary"
          >
            <Plus size={14} />
            Add Project
          </button>
        </div>
      </header>

      {/* Main List Area */}
      <ListView
        orgId={orgId}
        projects={projects}
        tasks={tasks}
        onTaskClick={openTaskModal}
        onTaskPatch={handleTaskPatch}
        onViewCompleted={handleViewCompleted}
        onTaskCreated={handleTaskCreated}
        onTaskDeleted={handleTaskDeleted}
        onProjectDeleted={handleProjectDeleted}
        onProjectUpdated={handleProjectUpdated}
        onProjectArchived={handleProjectArchived}
        onProjectsReordered={handleProjectsReordered}
        isProjectOrderSaving={projectOrderSaving}
      />

      {/* Completed Tasks Modal */}
      {completedTasksProject && (
        <div
          className="modal-overlay"
          onClick={(e) => e.target.classList.contains('modal-overlay') && setCompletedTasksProject(null)}
        >
          <div className="modal-content task-history-modal" style={{ width: '600px', maxHeight: '80vh', display: 'flex', flexDirection: 'column' }}>
            <div className="task-history-modal-header" style={{
              padding: '20px 24px',
              borderBottom: '1px solid var(--border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              background: 'var(--surface-alt)'
            }}>
              <div className="task-history-modal-title-wrap" style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <History size={18} color="var(--text-muted)" />
                <h3 className="task-history-modal-title" style={{ fontSize: '14px', fontWeight: '700', letterSpacing: '-0.02em' }}>
                  Completed Tasks: {completedTasksProject.name}
                </h3>
              </div>
              <button onClick={() => setCompletedTasksProject(null)} className="btn-ghost" style={{ padding: '6px' }}>
                <X size={16} />
              </button>
            </div>

            <div className="task-history-modal-body" style={{ overflowY: 'auto', padding: '24px', flexGrow: 1, display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {tasks
                .filter(t => t.project_id === completedTasksProject.id && t.status === 'Done')
                .sort((a, b) => {
                  const dateA = a.completed_at || a.updated_at || a.created_at
                  const dateB = b.completed_at || b.updated_at || b.created_at
                  return new Date(dateB) - new Date(dateA)
                })
                .map(task => (
                  <div key={task.id} className="task-history-row" style={{
                    padding: '12px 16px',
                    background: 'var(--surface)',
                    border: '1px solid var(--border-strong)',
                    borderRadius: '8px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    opacity: 0.7
                  }}>
                    <div className="task-history-row-main" style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                      <Check size={16} color="var(--success)" />
                      <div>
                        <div className="task-history-task-title" style={{ fontSize: '13px', fontWeight: '500', color: 'var(--text-disabled)', textDecoration: 'line-through' }}>{task.summary}</div>
                        <button
                          type="button"
                          onClick={() => openCompletedAtEditor(task)}
                          title="Edit completed date"
                          aria-label={`Edit completed date for ${task.summary}`}
                          style={{
                            marginTop: '2px',
                            padding: 0,
                            border: 0,
                            background: 'transparent',
                            color: 'var(--accent)',
                            fontSize: '10px',
                            fontFamily: 'var(--font-mono)',
                            cursor: 'pointer',
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '5px'
                          }}
                        >
                          <CalendarClock size={11} />
                          {formatCompletedAt(task.completed_at || task.created_at)}
                        </button>
                      </div>
                    </div>
                    <div className="task-history-actions" style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                      <button
                        onClick={() => openTaskModal(task)}
                        className="btn-ghost"
                        style={{ padding: '6px', color: 'var(--text-muted)' }}
                        title="Open full details"
                      >
                        <Maximize2 size={14} />
                      </button>
                      <button
                        onClick={() => handleRestoreTask(task.id)}
                        className="btn-ghost"
                        style={{ padding: '6px 12px', fontSize: '11px', gap: '6px', color: 'var(--accent)' }}
                      >
                        <RefreshCcw size={12} />
                        Restore
                      </button>
                    </div>
                  </div>
                ))
              }
              {tasks.filter(t => t.project_id === completedTasksProject.id && t.status === 'Done').length === 0 && (
                <div className="task-history-empty" style={{ textAlign: 'center', color: 'var(--text-disabled)', fontSize: '12px', padding: '32px' }}>
                  No completed tasks found in archive.
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Completed Date Editor */}
      {completedAtEditor && (
        <div
          className="modal-overlay"
          onClick={(e) => e.target.classList.contains('modal-overlay') && !completedAtSaving && setCompletedAtEditor(null)}
        >
          <div className="modal-content" style={{ maxWidth: '460px', padding: 0, overflow: 'hidden' }}>
            <div style={{
              padding: '20px 24px',
              borderBottom: '1px solid var(--border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              background: 'var(--surface-alt)'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px', minWidth: 0 }}>
                <div style={{
                  width: '32px', height: '32px', borderRadius: '8px',
                  background: 'var(--accent-muted)', border: '1px solid var(--accent-subtle)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  flexShrink: 0
                }}>
                  <CalendarClock size={16} color="var(--accent)" />
                </div>
                <div style={{ minWidth: 0 }}>
                  <h3 style={{ fontSize: '14px', fontWeight: '700', letterSpacing: '-0.02em', color: 'var(--text)' }}>
                    Edit Completed Date
                  </h3>
                  <div style={{
                    fontSize: '11px',
                    color: 'var(--text-muted)',
                    marginTop: '4px',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap'
                  }}>
                    {completedAtEditor.task.summary}
                  </div>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setCompletedAtEditor(null)}
                disabled={completedAtSaving}
                className="btn-ghost"
                style={{ padding: '6px' }}
                aria-label="Close completed date editor"
              >
                <X size={16} />
              </button>
            </div>

            <div style={{ padding: '28px 24px' }}>
              <label style={{
                display: 'block',
                fontSize: '10px',
                fontWeight: '800',
                color: 'var(--text-disabled)',
                textTransform: 'uppercase',
                letterSpacing: '0.15em',
                marginBottom: '10px'
              }}>
                Completed at
              </label>
              <input
                autoFocus
                type="datetime-local"
                value={completedAtEditor.value}
                onChange={(e) => setCompletedAtEditor(prev => ({ ...prev, value: e.target.value }))}
                onKeyDown={(e) => e.key === 'Enter' && handleSaveCompletedAt()}
                style={{
                  fontSize: '14px',
                  fontWeight: '600',
                  fontFamily: 'var(--font-mono)',
                  padding: '12px 16px',
                  background: 'var(--background)',
                  border: '1px solid var(--border-strong)',
                  width: '100%'
                }}
              />
            </div>

            <div style={{
              padding: '18px 24px',
              borderTop: '1px solid var(--border)',
              display: 'flex',
              justifyContent: 'flex-end',
              gap: '12px',
              background: 'var(--surface-alt)'
            }}>
              <button
                type="button"
                onClick={() => setCompletedAtEditor(null)}
                disabled={completedAtSaving}
                className="btn-ghost"
                style={{ fontSize: '12px' }}
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleSaveCompletedAt}
                disabled={completedAtSaving || !completedAtEditor.value}
                className="btn-primary"
                style={{ fontSize: '12px', padding: '8px 18px' }}
              >
                {completedAtSaving ? <Loader2 size={13} className="animate-spin" /> : <Check size={13} />}
                Save Date
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Archived Projects Modal */}
      {isArchiveModalOpen && (
        <div
          className="modal-overlay"
          onClick={(e) => e.target.classList.contains('modal-overlay') && setIsArchiveModalOpen(false)}
        >
          <div className="modal-content" style={{ width: '680px', maxHeight: '80vh', display: 'flex', flexDirection: 'column' }}>
            <div style={{
              padding: '20px 24px',
              borderBottom: '1px solid var(--border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              background: 'var(--surface-alt)'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <Archive size={18} color="var(--text-muted)" />
                <h3 style={{ fontSize: '14px', fontWeight: '700', letterSpacing: '-0.02em' }}>
                  Archived Projects: {org?.name}
                </h3>
              </div>
              <button onClick={() => setIsArchiveModalOpen(false)} className="btn-ghost" style={{ padding: '6px' }}>
                <X size={16} />
              </button>
            </div>

            <div style={{ overflowY: 'auto', padding: '24px', flexGrow: 1, display: 'flex', flexDirection: 'column', gap: '12px' }}>
              {archivedLoading ? (
                <div style={{ display: 'flex', justifyContent: 'center', padding: '32px', color: 'var(--text-muted)' }}>
                  <Loader2 className="animate-spin" size={18} />
                </div>
              ) : archivedError ? (
                <div style={{
                  padding: '18px',
                  border: '1px solid var(--error)',
                  borderRadius: '8px',
                  color: 'var(--error)',
                  background: 'var(--error-muted)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: '12px'
                }}>
                  <span style={{ fontSize: '12px' }}>{archivedError}</span>
                  <button onClick={fetchArchivedProjects} className="btn-ghost" style={{ fontSize: '11px', padding: '6px 10px' }}>
                    Retry
                  </button>
                </div>
              ) : archivedProjects.length === 0 ? (
                <div style={{ textAlign: 'center', color: 'var(--text-disabled)', fontSize: '12px', padding: '36px' }}>
                  No archived projects found.
                </div>
              ) : (
                archivedProjects.map(project => (
                  <div key={project.id} style={{
                    padding: '16px 18px',
                    background: 'var(--surface)',
                    border: '1px solid var(--border-strong)',
                    borderRadius: '8px',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: '16px'
                  }}>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontSize: '14px', fontWeight: '700', color: 'var(--text)', marginBottom: '4px' }}>
                        {project.name}
                      </div>
                      <div style={{
                        fontSize: '12px',
                        color: 'var(--text-muted)',
                        lineHeight: '1.5',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical'
                      }}>
                        {project.goal || project.description_markdown || 'No description available.'}
                      </div>
                      <div style={{ fontSize: '10px', color: 'var(--text-disabled)', fontFamily: 'var(--font-mono)', marginTop: '8px' }}>
                        Archived {project.archived_at ? new Date(project.archived_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' }) : 'date unknown'}
                      </div>
                    </div>
                    <button
                      onClick={() => handleRestoreProject(project.id)}
                      disabled={restoringProjectId === project.id}
                      className="btn-ghost"
                      style={{ padding: '7px 12px', fontSize: '11px', color: 'var(--accent)', flexShrink: 0 }}
                    >
                      {restoringProjectId === project.id ? <Loader2 size={13} className="animate-spin" /> : <ArchiveRestore size={13} />}
                      Restore
                    </button>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      )}

      {/* Create Project Modal */}
      {isProjectModalOpen && (
        <div
          className="modal-overlay"
          onClick={(e) => e.target.classList.contains('modal-overlay') && setIsProjectModalOpen(false)}
          onKeyDown={(e) => e.key === 'Escape' && setIsProjectModalOpen(false)}
        >
          <div className="modal-content" style={{ maxWidth: '480px', padding: '0', overflow: 'hidden' }}>
            <div style={{
              padding: '20px 24px',
              borderBottom: '1px solid var(--border)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              background: 'var(--surface-alt)'
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                <div style={{
                  width: '32px', height: '32px', borderRadius: '8px',
                  background: 'var(--accent-muted)', border: '1px solid var(--accent-subtle)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center'
                }}>
                  <Layers size={16} color="var(--accent)" />
                </div>
                <h3 style={{ fontSize: '14px', fontWeight: '700', letterSpacing: '-0.02em', color: 'var(--text)' }}>
                  Create Project
                </h3>
              </div>
              <button onClick={() => setIsProjectModalOpen(false)} className="btn-ghost" style={{ padding: '6px' }}>
                <X size={16} />
              </button>
            </div>

            <div style={{ padding: '32px 24px', display: 'flex', flexDirection: 'column', gap: '20px' }}>
              <div>
                <label style={{
                  display: 'block', fontSize: '10px', fontWeight: '800',
                  color: 'var(--text-disabled)', textTransform: 'uppercase',
                  letterSpacing: '0.15em', marginBottom: '10px'
                }}>
                  Project name
                </label>
                <input
                  autoFocus
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && handleConfirmCreateProject()}
                  placeholder="e.g. Website refresh"
                  style={{
                    fontSize: '14px',
                    fontWeight: '600',
                    fontFamily: 'var(--font-mono)',
                    padding: '12px 16px',
                    background: 'var(--background)',
                    border: '1px solid var(--border-strong)',
                    width: '100%'
                  }}
                />
                <div style={{ fontSize: '11px', color: 'var(--text-disabled)', marginTop: '8px', lineHeight: '1.4' }}>
                  Create a focused project stream for related tasks and assets.
                </div>
              </div>
            </div>

            <div style={{
              padding: '20px 24px',
              borderTop: '1px solid var(--border)',
              display: 'flex',
              justifyContent: 'flex-end',
              gap: '12px',
              background: 'var(--surface-alt)'
            }}>
              <button onClick={() => setIsProjectModalOpen(false)} className="btn-ghost" style={{ fontSize: '12px' }}>
                Cancel
              </button>
              <button
                onClick={handleConfirmCreateProject}
                disabled={!newProjectName.trim()}
                className="btn-primary"
                style={{ fontSize: '12px', padding: '8px 20px' }}
              >
                Create Project
              </button>
            </div>
          </div>
        </div>
      )}

      {isTaskModalOpen && (
        <TaskModal
          task={selectedTask}
          onClose={handleCloseModal}
          onTaskUpdated={() => handleTaskDeleted(selectedTask?.id)}
          onTaskPatch={handleTaskPatch}
          projectContext={projects.find(p => p.id === selectedTask?.project_id)}
        />
      )}
    </div>
  )
}

function X({ size }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <line x1="18" y1="6" x2="6" y2="18"></line>
      <line x1="6" y1="6" x2="18" y2="18"></line>
    </svg>
  )
}
