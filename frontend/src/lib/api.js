const API_BASE = '/api/v1'

function getApiKey() {
  return localStorage.getItem('tf_api_key') || 'default'
}

async function request(path, opts = {}) {
  const apiKey = getApiKey()
  const url = `${API_BASE}${path}`
  const res = await fetch(url, {
    headers: {
      'Content-Type': 'application/json',
      'X-API-Key': apiKey,
      ...opts.headers,
    },
    ...opts,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `HTTP ${res.status}`)
  }
  if (res.status === 204) return null
  return res.json()
}

export const api = {
  submitJob(data) {
    return request('/jobs', { method: 'POST', body: JSON.stringify(data) })
  },
  listJobs(params = {}) {
    const qs = new URLSearchParams(params).toString()
    return request(`/jobs${qs ? '?' + qs : ''}`)
  },
  getJob(id) {
    return request(`/jobs/${id}`)
  },
  cancelJob(id) {
    return request(`/jobs/${id}/cancel`, { method: 'POST' })
  },
  retryJob(id) {
    return request(`/jobs/${id}/retry`, { method: 'POST' })
  },
  queueStatus() {
    return request('/queue/status')
  },
  pauseQueue() {
    return request('/queue/pause', { method: 'POST' })
  },
  resumeQueue() {
    return request('/queue/resume', { method: 'POST' })
  },
  listDLQ() {
    return request('/dlq')
  },
  replayDLQ(id) {
    return request(`/dlq/${id}/replay`, { method: 'POST' })
  },
  createDAG(data) {
    return request('/dags', { method: 'POST', body: JSON.stringify(data) })
  },
  listDAGs() {
    return request('/dags')
  },
  getDAG(id) {
    return request(`/dags/${id}`)
  },
  executeDAG(id) {
    return request(`/dags/${id}/execute`, { method: 'POST' })
  },
  listCron() {
    return request('/cron')
  },
  createCron(data) {
    return request('/cron', { method: 'POST', body: JSON.stringify(data) })
  },
  deleteCron(id) {
    return request(`/cron/${id}`, { method: 'DELETE' })
  },
  health() {
    return fetch('/health').then(r => r.json())
  },
}
