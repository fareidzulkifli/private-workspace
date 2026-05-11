'use client';

import { useNavigate } from 'react-router-dom'
import { logout } from '@/lib/api'

export default function LogoutButton({ style }) {
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login', { replace: true })
  }

  return (
    <button
      onClick={handleLogout}
      style={{
        background: 'transparent',
        border: '1px solid var(--border)',
        fontSize: '12px',
        padding: '6px 12px',
        ...style
      }}
    >
      Logout
    </button>
  )
}
