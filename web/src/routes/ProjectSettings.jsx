import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import api from '../lib/api'

export default function ProjectSettings() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [project, setProject] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    let mounted = true
    setLoading(true)
    api.get(`/api/projects/${id}`)
      .then(data => { if (mounted) setProject(data) })
      .catch(err => { if (mounted) setError(err.message) })
      .finally(() => { if (mounted) setLoading(false) })
    return () => { mounted = false }
  }, [id])

  const handleSave = async (event) => {
    event.preventDefault()
    try {
      setSaving(true)
      await api.patch(`/api/projects/${id}`, project)
      navigate(-1)
    } catch (err) {
      alert('Error saving project settings: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!confirm('Are you sure you want to delete this project? This will delete all tasks within it.')) return
    try {
      setSaving(true)
      await api.delete(`/api/projects/${id}`)
      navigate('/dashboard')
    } catch (err) {
      alert('Error deleting project: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <div style={{ padding: '48px', color: 'var(--text-muted)' }}>Loading settings...</div>
  if (error) return <div style={{ padding: '48px', color: 'var(--error)' }}>Error: {error}</div>

  return (
    <div style={{ maxWidth: '780px', margin: '0 auto', padding: '48px' }}>
      <header style={{ marginBottom: '36px', display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h1 style={{ fontSize: '22px', letterSpacing: '-0.03em', marginBottom: '4px' }}>Project Settings</h1>
          <p style={{ color: 'var(--text-muted)', fontSize: '13px' }}>Configure context and AI behavior for "{project?.name}"</p>
        </div>
        <button onClick={() => navigate(-1)} className="btn-ghost">Back to Board</button>
      </header>

      <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
        <div className="card" style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
          <div style={{ display: 'flex', gap: '20px' }}>
            <div style={{ flexGrow: 1 }}>
              <label className="form-label">Project Name</label>
              <input value={project?.name || ''} onChange={e => setProject({ ...project, name: e.target.value })} required />
            </div>
            <div style={{ flexGrow: 1 }}>
              <label className="form-label">Project Type</label>
              <select value={project?.project_type || 'Work'} onChange={e => setProject({ ...project, project_type: e.target.value })}>
                <option value="Work">Work</option>
                <option value="Personal">Personal</option>
                <option value="Learning">Learning</option>
                <option value="Creative">Creative</option>
                <option value="Admin">Admin</option>
              </select>
            </div>
          </div>

          <div>
            <label className="form-label">Target Completion Date</label>
            <input type="date" value={project?.target_date || ''} onChange={e => setProject({ ...project, target_date: e.target.value || null })} />
          </div>

          <div>
            <label className="form-label">High-Level Goal</label>
            <input value={project?.goal || ''} onChange={e => setProject({ ...project, goal: e.target.value })} />
          </div>

          <div>
            <label className="form-label">Description (Markdown)</label>
            <textarea rows="4" value={project?.description_markdown || ''} onChange={e => setProject({ ...project, description_markdown: e.target.value })} />
          </div>

          <div>
            <label className="form-label">Detailed Context (Markdown)</label>
            <textarea rows="6" value={project?.context_markdown || ''} onChange={e => setProject({ ...project, context_markdown: e.target.value })} />
          </div>

          <div>
            <label className="form-label">Current Focus</label>
            <input value={project?.current_focus || ''} onChange={e => setProject({ ...project, current_focus: e.target.value })} />
          </div>

          <div>
            <label className="form-label">Custom AI Instructions</label>
            <textarea rows="4" value={project?.ai_instructions || ''} onChange={e => setProject({ ...project, ai_instructions: e.target.value })} />
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <button type="button" onClick={handleDelete} className="btn-danger" disabled={saving}>Delete Project</button>
          <button type="submit" disabled={saving}>{saving ? 'Saving...' : 'Save Settings'}</button>
        </div>
      </form>
    </div>
  )
}
