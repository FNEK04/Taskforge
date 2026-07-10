import { writable } from 'svelte/store'

export const page = writable('dashboard')
export const pageParams = writable({})
export const liveEvents = writable([])
export const queueStats = writable({ paused: false, stream_len: 0, pending: 0 })
export const toasts = writable([])

export function navigate(to, params = {}) {
  page.set(to)
  pageParams.set(params)
}

export function toast(msg, type = 'success') {
  const id = Date.now()
  toasts.update(t => [...t, { id, msg, type }])
  setTimeout(() => toasts.update(t => t.filter(x => x.id !== id)), 4000)
}

export const STATUS_COLORS = {
  pending: '#f59e0b',
  scheduled: '#a78bfa',
  queued: '#3b82f6',
  running: '#06b6d4',
  completed: '#22c55e',
  failed: '#ef4444',
  cancelled: '#6b7280',
  dlq: '#dc2626',
}

export const STATUS_LABELS = {
  pending: 'Ожидает',
  scheduled: 'Запланирована',
  queued: 'В очереди',
  running: 'Выполняется',
  completed: 'Завершена',
  failed: 'Ошибка',
  cancelled: 'Отменена',
  dlq: 'В DLQ',
}

export const STATUS_ICONS = {
  pending: 'fa-regular fa-clock',
  scheduled: 'fa-regular fa-calendar',
  queued: 'fa-regular fa-circle-down',
  running: 'fa-solid fa-gear',
  completed: 'fa-regular fa-circle-check',
  failed: 'fa-regular fa-circle-xmark',
  cancelled: 'fa-regular fa-circle-stop',
  dlq: 'fa-solid fa-skull',
}

export const EVENT_ICONS = {
  'job.created': 'fa-regular fa-circle-down',
  'job.started': 'fa-solid fa-play',
  'job.completed': 'fa-regular fa-circle-check',
  'job.failed': 'fa-regular fa-circle-xmark',
  'job.retry': 'fa-solid fa-rotate',
  'job.dlq': 'fa-solid fa-skull',
}

export function eventIcon(type) {
  return EVENT_ICONS[type] || 'fa-regular fa-bell'
}

export function fmtTime(t) {
  if (!t) return '—'
  return new Date(t).toLocaleString('ru')
}

export function ago(t) {
  if (!t) return ''
  const diff = Date.now() - new Date(t).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'только что'
  if (mins < 60) return `${mins} мин. назад`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs} ч. назад`
  const days = Math.floor(hrs / 24)
  return `${days} дн. назад`
}
