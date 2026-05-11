import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import Board from '../components/Board'
import api from '../lib/api'

export default function OrgBoard() {
  const { slug } = useParams()
  const [orgId, setOrgId] = useState(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let mounted = true
    setError('')
    setOrgId(null)
    api.get(`/api/orgs/by-slug/${encodeURIComponent(slug)}`)
      .then(org => {
        if (mounted) setOrgId(org.id)
      })
      .catch(err => {
        if (mounted) setError(err.message)
      })
    return () => { mounted = false }
  }, [slug])

  if (error) return <div style={{ padding: '48px', color: 'var(--error)' }}>Organization not found</div>
  if (!orgId) return <div style={{ padding: '48px', color: 'var(--text-muted)' }}>Loading workspace...</div>
  return <div className="org-page"><Board orgId={orgId} /></div>
}
