let csrfToken = null
let session = { authenticated: false }

const unsafeMethods = new Set(['POST', 'PATCH', 'PUT', 'DELETE'])

async function request(path, options = {}) {
  const method = (options.method || 'GET').toUpperCase()
  const headers = new Headers(options.headers || {})
  const body = options.body

  if (body !== undefined && !(body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (unsafeMethods.has(method) && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }

  const res = await fetch(path, {
    ...options,
    method,
    headers,
    credentials: 'include',
    body: body !== undefined && !(body instanceof FormData) && typeof body !== 'string'
      ? JSON.stringify(body)
      : body,
  })

  const contentType = res.headers.get('Content-Type') || ''
  const data = contentType.includes('application/json') ? await res.json() : await res.text()

  if (res.status === 401 && !window.location.pathname.startsWith('/login')) {
    window.history.replaceState(null, '', '/login')
    window.dispatchEvent(new PopStateEvent('popstate'))
  }

  if (!res.ok) {
    const message = data?.error || res.statusText || 'Request failed'
    const error = new Error(message)
    error.status = res.status
    error.data = data
    throw error
  }

  return data
}

export async function apiFetch(input, init = {}) {
  const url = typeof input === 'string' ? input : input?.url || ''
  const isAPI = url.startsWith('/api/')
  if (!isAPI) {
    return fetch(input, init)
  }
  const method = (init.method || 'GET').toUpperCase()
  const headers = new Headers(init.headers || {})
  if (unsafeMethods.has(method) && csrfToken) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const res = await fetch(input, {
    ...init,
    headers,
    credentials: 'include',
  })
  if (res.status === 401 && !window.location.pathname.startsWith('/login')) {
    window.history.replaceState(null, '', '/login')
    window.dispatchEvent(new PopStateEvent('popstate'))
  }
  return res
}

export function installAPIFetch() {
  const nativeFetch = window.fetch.bind(window)
  window.fetch = async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input?.url || ''
    if (!url.startsWith('/api/')) {
      return nativeFetch(input, init)
    }
    const method = (init.method || 'GET').toUpperCase()
    const headers = new Headers(init.headers || {})
    if (unsafeMethods.has(method) && csrfToken) {
      headers.set('X-CSRF-Token', csrfToken)
    }
    const res = await nativeFetch(input, {
      ...init,
      headers,
      credentials: 'include',
    })
    if (res.status === 401 && !window.location.pathname.startsWith('/login')) {
      window.history.replaceState(null, '', '/login')
      window.dispatchEvent(new PopStateEvent('popstate'))
    }
    return res
  }
}

export async function refreshSession() {
  session = await request('/api/auth/session')
  csrfToken = session.csrfToken || null
  return session
}

export async function login(email, password) {
  const result = await request('/api/auth/login', {
    method: 'POST',
    body: { email, password },
  })
  csrfToken = result.csrfToken || null
  session = { authenticated: true, user: result.user, csrfToken }
  window.dispatchEvent(new Event('authChanged'))
  return result
}

export async function logout() {
  const result = await request('/api/auth/logout', { method: 'POST' })
  csrfToken = null
  session = { authenticated: false }
  window.dispatchEvent(new Event('authChanged'))
  return result
}

export function getSession() {
  return session
}

export const api = {
  get: (path, options) => request(path, { ...options, method: 'GET' }),
  post: (path, body, options) => request(path, { ...options, method: 'POST', body }),
  patch: (path, body, options) => request(path, { ...options, method: 'PATCH', body }),
  delete: (path, options) => request(path, { ...options, method: 'DELETE' }),
  fetch: apiFetch,
  request,
}

export default api
