import { useEffect, useState } from 'react'
import { useLocation } from 'react-router-dom'
import DashboardView from '../components/DashboardView'
import api from '../lib/api'

export default function Dashboard() {
  const location = useLocation()
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)

  useEffect(() => {
    let mounted = true
    setError(null)
    api.get(`/api/dashboard${location.search}`)
      .then(result => {
        if (mounted) setData(result)
      })
      .catch(err => {
        if (mounted) setError(err.message)
      })
    return () => { mounted = false }
  }, [location.search])

  if (error) {
    return <DashboardView data={{ error, empty: true, kpis: [], events: [] }} />
  }
  if (!data) {
    return <div style={{ padding: '48px', color: 'var(--text-muted)' }}>Loading dashboard...</div>
  }
  return <DashboardView key={data.calendar?.monthKey || location.search} data={data} />
}
