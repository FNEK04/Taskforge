<script>
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api.js'
  import { navigate, liveEvents, eventIcon, ago } from '../lib/stores.js'
  import StatusBadge from '../components/StatusBadge.svelte'

  let stats = { total: 0, queued: 0, running: 0, completed: 0, failed: 0, dlq: 0 }
  let recentJobs = []
  let events = []
  let eventsUnsub
  let qs = {}
  let refreshInterval

  eventsUnsub = liveEvents.subscribe(v => events = v)

  async function load() {
    try {
      const allJobs = await api.listJobs({ limit: 50 })
      stats.total = allJobs.length
      stats.queued = allJobs.filter(j => j.status === 'queued').length
      stats.running = allJobs.filter(j => j.status === 'running').length
      stats.completed = allJobs.filter(j => j.status === 'completed').length
      stats.failed = allJobs.filter(j => j.status === 'failed').length
      stats.dlq = allJobs.filter(j => j.status === 'dlq').length
      recentJobs = allJobs.slice(0, 8)
    } catch (_) {}
    try {
      qs = await api.queueStatus()
    } catch (_) {}
  }

  onMount(() => {
    load()
    refreshInterval = setInterval(load, 8000)
  })

  onDestroy(() => {
    if (eventsUnsub) eventsUnsub()
    if (refreshInterval) clearInterval(refreshInterval)
  })

  $: active = stats.queued + stats.running
  $: done = stats.completed
  $: problems = stats.failed + stats.dlq
  $: total = stats.total || 1
  $: activePct = (active / total * 100).toFixed(0)
  $: donePct = (done / total * 100).toFixed(0)
  $: problemPct = (problems / total * 100).toFixed(0)
</script>

<div class="dashboard animate-in">
  <div class="header">
    <div>
      <h1 class="page-title">Главная</h1>
      <p class="page-subtitle">Состояние очереди задач <span class="mono" style="color:#60a5fa;">TaskForge</span></p>
    </div>
    <div class="header-actions">
      <button class="btn-ghost btn-sm" on:click={load} title="Обновить данные">
        <i class="fa-solid fa-rotate"></i>
      </button>
      <button class="btn-primary" on:click={() => navigate('submit-job')}>
        <i class="fa-solid fa-plus"></i> Создать задачу
      </button>
    </div>
  </div>

  <div class="stat-cards">
    <div class="stat-card active">
      <div class="stat-icon-wrap" style="background:#3b82f61a;"><i class="fa-solid fa-bolt" style="color:#3b82f6;"></i></div>
      <div class="stat-info">
        <span class="stat-num">{active}</span>
        <span class="stat-lbl">Активные задачи</span>
        <span class="stat-desc">В очереди и выполняются</span>
      </div>
    </div>
    <div class="stat-card success">
      <div class="stat-icon-wrap" style="background:#22c55e1a;"><i class="fa-regular fa-circle-check" style="color:#22c55e;"></i></div>
      <div class="stat-info">
        <span class="stat-num">{done}</span>
        <span class="stat-lbl">Завершённые</span>
        <span class="stat-desc">Успешно обработаны</span>
      </div>
    </div>
    <div class="stat-card danger">
      <div class="stat-icon-wrap" style="background:#ef44441a;"><i class="fa-solid fa-triangle-exclamation" style="color:#ef4444;"></i></div>
      <div class="stat-info">
        <span class="stat-num">{problems}</span>
        <span class="stat-lbl">Проблемы</span>
        <span class="stat-desc">Ошибки и DLQ</span>
      </div>
    </div>
    <div class="stat-card total">
      <div class="stat-icon-wrap" style="background:#a78bfa1a;"><i class="fa-solid fa-layer-group" style="color:#a78bfa;"></i></div>
      <div class="stat-info">
        <span class="stat-num">{stats.total}</span>
        <span class="stat-lbl">Всего задач</span>
        <span class="stat-desc">За всё время</span>
      </div>
    </div>
  </div>

  <div class="progress-section">
    <div class="progress-bar">
      <div class="progress-fill active" style="width: {activePct}%"></div>
      <div class="progress-fill success" style="width: {donePct}%"></div>
      <div class="progress-fill danger" style="width: {problemPct}%"></div>
    </div>
    <div class="progress-legend">
      <span><span class="dot active"></span> Активные ({active})</span>
      <span><span class="dot success"></span> Завершённые ({done})</span>
      <span><span class="dot danger"></span> С ошибками ({problems})</span>
      <span class="auto-refresh" title="Автообновление каждые 8 секунд"><i class="fa-solid fa-rotate" style="font-size:10px;"></i> авто</span>
    </div>
  </div>

  <div class="grid-2">
    <div class="card">
      <div class="card-title"><i class="fa-solid fa-list"></i> Последние задачи</div>
      {#if recentJobs.length === 0}
        <div class="empty"><i class="fa-regular fa-inbox"></i> Пока нет задач</div>
      {:else}
        {#each recentJobs as job}
          <div class="job-item" on:click={() => navigate('job-detail', { id: job.id })} role="button" tabindex="0" on:keydown={(e) => e.key === 'Enter' && navigate('job-detail', { id: job.id })}>
            <div class="job-left">
              <StatusBadge status={job.status} />
              <span class="job-type">{job.type}</span>
            </div>
            <div class="job-right">
              <span class="mono" style="color:#64748b;">{job.id?.slice(0, 10)}</span>
              <span class="text-muted" style="font-size:11px;">{ago(job.created_at)}</span>
            </div>
          </div>
        {/each}
        <button class="btn-ghost btn-sm" style="margin-top: 8px; width: 100%;" on:click={() => navigate('jobs')}>
          Все задачи <i class="fa-solid fa-arrow-right" style="font-size:10px;"></i>
        </button>
      {/if}
    </div>

    <div class="card">
      <div class="card-title"><i class="fa-solid fa-wave-square"></i> Активность в реальном времени</div>
      {#if events.length === 0}
        <div class="empty">
          <i class="fa-solid fa-plug" style="font-size:24px;display:block;margin-bottom:8px;color:#334155;"></i>
          Ожидание событий через WebSocket...
        </div>
      {:else}
        <div class="events-list">
          {#each events.slice(0, 20) as ev}
            <div class="event-item">
              <span class="event-dot" style="background:{ev.type.includes('completed') ? '#22c55e' : ev.type.includes('failed') ? '#ef4444' : ev.type.includes('started') ? '#06b6d4' : '#3b82f6'}"></span>
              <i class="{eventIcon(ev.type)}" style="color:#64748b;font-size:11px;width:14px;"></i>
              <span class="event-label">{ev.type.replace('job.', '')}</span>
              <span class="mono text-muted">{ev.payload?.id?.slice(0, 10)}</span>
              <span class="event-time">{ago(ev.time)}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 20px; }
  .header-actions { display: flex; gap: 8px; align-items: center; }
  .stat-cards { display: grid; grid-template-columns: repeat(4, 1fr); gap: 14px; margin-bottom: 20px; }
  .stat-card {
    display: flex;
    align-items: center;
    gap: 14px;
    background: #1e293b;
    border-radius: 12px;
    padding: 18px;
    border: 1px solid #334155;
    transition: border-color .2s, transform .15s;
  }
  .stat-card:hover { border-color: #3b82f633; transform: translateY(-1px); }
  .stat-icon-wrap {
    width: 44px;
    height: 44px;
    border-radius: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 18px;
    flex-shrink: 0;
  }
  .stat-info { display: flex; flex-direction: column; }
  .stat-num { font-size: 26px; font-weight: 800; line-height: 1; }
  .stat-lbl { font-size: 12px; color: #94a3b8; margin-top: 2px; }
  .stat-desc { font-size: 10px; color: #475569; margin-top: 1px; }
  .progress-section { margin-bottom: 24px; }
  .progress-bar {
    height: 8px;
    background: #0f172a;
    border-radius: 4px;
    overflow: hidden;
    display: flex;
    gap: 2px;
  }
  .progress-fill { height: 100%; border-radius: 4px; transition: width .6s ease; }
  .progress-fill.active { background: #3b82f6; }
  .progress-fill.success { background: #22c55e; }
  .progress-fill.danger { background: #ef4444; }
  .progress-legend { display: flex; gap: 16px; margin-top: 8px; font-size: 12px; color: #94a3b8; align-items: center; }
  .auto-refresh { margin-left: auto; font-size: 11px; color: #475569; }
  .dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 4px; vertical-align: middle; }
  .dot.active { background: #3b82f6; }
  .dot.success { background: #22c55e; }
  .dot.danger { background: #ef4444; }
  .grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
  .job-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 0;
    border-bottom: 1px solid #0f172a;
    cursor: pointer;
    transition: background .1s, padding-left .1s;
    border-radius: 4px;
    padding-left: 4px;
    padding-right: 4px;
  }
  .job-item:last-child { border-bottom: none; }
  .job-item:hover { background: #0f172a; padding-left: 8px; }
  .job-left { display: flex; align-items: center; gap: 10px; }
  .job-right { display: flex; align-items: center; gap: 12px; }
  .job-type { font-weight: 500; font-size: 13px; }
  .events-list { display: flex; flex-direction: column; gap: 4px; max-height: 420px; overflow-y: auto; }
  .event-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    background: #0f172a;
    border-radius: 6px;
    font-size: 12px;
    transition: background .1s;
  }
  .event-item:hover { background: #1a2332; }
  .event-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0; }
  .event-label { font-weight: 600; color: #94a3b8; text-transform: capitalize; }
  .event-time { margin-left: auto; color: #475569; font-size: 11px; }
</style>
