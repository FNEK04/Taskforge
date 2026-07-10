<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import { navigate, toast, fmtTime } from '../lib/stores.js'
  import StatusBadge from '../components/StatusBadge.svelte'

  export let id = ''
  let job = null
  let loading = true

  onMount(() => load())

  async function load() {
    if (!id) return
    loading = true
    try {
      job = await api.getJob(id)
    } catch (_) { job = null }
    loading = false
  }

  async function cancel() {
    if (!confirm('Отменить эту задачу?')) return
    try {
      await api.cancelJob(id)
      toast('Задача отменена')
      load()
    } catch (_) {}
  }

  async function retry() {
    try {
      await api.retryJob(id)
      toast('Задача отправлена на повтор')
      load()
    } catch (_) {}
  }

  function renderPayload(payload) {
    if (!payload) return '{}'
    try {
      return JSON.stringify(typeof payload === 'string' ? JSON.parse(payload) : payload, null, 2)
    } catch {
      return typeof payload === 'string' ? payload : JSON.stringify(payload)
    }
  }
</script>

<div class="detail-page animate-in">
  <div class="breadcrumb">
    <button class="link" on:click={() => navigate('jobs')}>
      <i class="fa-solid fa-arrow-left"></i> Все задачи
    </button>
    <span>/ {id?.slice(0, 12)}</span>
  </div>

  {#if loading}
    <div class="empty"><i class="fa-solid fa-spinner fa-spin"></i> Загрузка...</div>
  {:else if !job}
    <div class="empty"><i class="fa-regular fa-circle-question"></i> Задача не найдена</div>
  {:else}
    <div class="card">
      <div class="detail-header">
        <div class="detail-title">
          <span class="mono" style="font-size:15px;font-weight:700;">{job.id?.slice(0, 16)}</span>
          {#if job.type}
            <span class="type-tag">{job.type}</span>
          {/if}
        </div>
        <StatusBadge status={job.status} />
      </div>

      <div class="detail-grid">
        <div class="field-item">
          <div class="field-label"><i class="fa-solid fa-tag"></i> Тип</div>
          <span class="field-val">{job.type || '—'}</span>
        </div>
        <div class="field-item">
          <div class="field-label"><i class="fa-solid fa-building"></i> Тенант</div>
          <span class="field-val">{job.tenant_id || '—'}</span>
        </div>
        <div class="field-item">
          <div class="field-label"><i class="fa-solid fa-arrow-up-wide-short"></i> Приоритет</div>
          <span class="field-val">
            <span class="priority" class:high={job.priority >= 7} class:mid={job.priority >= 3 && job.priority < 7}>
              {job.priority}
            </span>
          </span>
        </div>
        <div class="field-item">
          <div class="field-label"><i class="fa-solid fa-rotate"></i> Попытки</div>
          <span class="field-val">{job.retry_count} / {job.max_retries}</span>
        </div>
        <div class="field-item">
          <div class="field-label"><i class="fa-regular fa-calendar-plus"></i> Создана</div>
          <span class="field-val">{fmtTime(job.created_at)}</span>
        </div>
        <div class="field-item">
          <div class="field-label"><i class="fa-solid fa-play"></i> Запущена</div>
          <span class="field-val">{fmtTime(job.started_at)}</span>
        </div>
        <div class="field-item">
          <div class="field-label"><i class="fa-regular fa-calendar-check"></i> Завершена</div>
          <span class="field-val">{fmtTime(job.completed_at)}</span>
        </div>
        <div class="field-item">
          <div class="field-label"><i class="fa-regular fa-clock"></i> Запланирована</div>
          <span class="field-val">{fmtTime(job.scheduled_at)}</span>
        </div>
      </div>

      {#if job.last_error}
        <div class="error-box">
          <div class="error-title"><i class="fa-solid fa-circle-exclamation"></i> Последняя ошибка</div>
          <pre class="error-text">{job.last_error}</pre>
        </div>
      {/if}

      {#if job.dag_run_id}
        <div class="meta-box">
          <i class="fa-solid fa-project-diagram" style="color:#64748b;"></i>
          <span class="meta-label">DAG Run:</span>
          <span class="mono">{job.dag_run_id}</span>
          {#if job.dag_id}
            <span class="meta-label">DAG ID:</span>
            <span class="mono">{job.dag_id}</span>
          {/if}
        </div>
      {/if}

      {#if job.idempotency_key}
        <div class="meta-box">
          <i class="fa-solid fa-fingerprint" style="color:#64748b;"></i>
          <span class="meta-label">Idempotency Key:</span>
          <span class="mono">{job.idempotency_key}</span>
        </div>
      {/if}

      <div class="payload-section">
        <div class="payload-label"><i class="fa-solid fa-cube"></i> Полезная нагрузка (Payload)</div>
        <pre class="payload-box">{renderPayload(job.payload)}</pre>
      </div>

      {#if job.status === 'queued' || job.status === 'running' || job.status === 'pending'}
        <div class="detail-actions">
          <button class="btn-danger" on:click={cancel}>
            <i class="fa-solid fa-ban"></i> Отменить задачу
          </button>
        </div>
      {:else if job.status === 'failed'}
        <div class="detail-actions">
          <button class="btn-primary" on:click={retry}>
            <i class="fa-solid fa-rotate"></i> Повторить
          </button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .detail-page { max-width: 800px; }
  .detail-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 16px;
    border-bottom: 1px solid #334155;
    margin-bottom: 16px;
  }
  .detail-title { display: flex; align-items: center; gap: 10px; }
  .type-tag {
    background: #1e3a5f;
    color: #93c5fd;
    padding: 2px 10px;
    border-radius: 12px;
    font-size: 12px;
    font-weight: 600;
  }
  .detail-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
    margin-bottom: 16px;
  }
  .field-item { display: flex; flex-direction: column; gap: 2px; }
  .field-label { font-size: 11px; color: #64748b; font-weight: 600; display: flex; align-items: center; gap: 6px; }
  .field-val { font-size: 13px; }
  .priority { font-weight: 700; }
  .priority.high { color: #ef4444; }
  .priority.mid { color: #f59e0b; }
  .error-box {
    background: #450a0a;
    border: 1px solid #7f1d1d;
    border-radius: 8px;
    padding: 12px;
    margin-bottom: 14px;
  }
  .error-title { font-weight: 600; color: #f87171; margin-bottom: 6px; display: flex; align-items: center; gap: 6px; }
  .error-text { font-size: 12px; color: #fca5a5; white-space: pre-wrap; font-family: inherit; }
  .meta-box {
    background: #0f172a;
    border-radius: 8px;
    padding: 10px 14px;
    margin-bottom: 14px;
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 12px;
    flex-wrap: wrap;
  }
  .meta-label { color: #64748b; font-weight: 600; }
  .payload-section { margin-bottom: 14px; }
  .payload-label { font-size: 12px; color: #94a3b8; font-weight: 600; margin-bottom: 6px; display: flex; align-items: center; gap: 6px; }
  .payload-box {
    background: #0f172a;
    border: 1px solid #334155;
    border-radius: 8px;
    padding: 14px;
    font-size: 12px;
    overflow-x: auto;
    white-space: pre-wrap;
    min-height: 60px;
  }
  .detail-actions { padding-top: 14px; border-top: 1px solid #334155; display: flex; gap: 10px; }
</style>
