<script>
  import { page, navigate, queueStats } from '../lib/stores.js'
  import { onMount, onDestroy } from 'svelte'
  import { api } from '../lib/api.js'

  let stats = {}
  let interval
  const unsub = queueStats.subscribe(v => stats = v)

  onMount(() => {
    api.queueStatus().then(s => queueStats.set(s)).catch(() => {})
    interval = setInterval(() => {
      api.queueStatus().then(s => queueStats.set(s)).catch(() => {})
    }, 5000)
  })

  onDestroy(() => {
    if (interval) clearInterval(interval)
    unsub()
  })

  const items = [
    { id: 'dashboard', label: 'Главная', icon: 'fa-solid fa-gauge-high' },
    { id: 'jobs', label: 'Задачи', icon: 'fa-solid fa-list-check' },
    { id: 'submit-job', label: 'Создать задачу', icon: 'fa-solid fa-plus-circle' },
    { id: 'dags', label: 'DAG-графы', icon: 'fa-solid fa-project-diagram' },
    { id: 'cron', label: 'Расписание', icon: 'fa-regular fa-clock' },
    { id: 'dlq', label: 'Бракованные', icon: 'fa-solid fa-triangle-exclamation' },
  ]
</script>

<aside class="sidebar">
  <div class="logo" on:click={() => navigate('dashboard')} on:keydown={(e) => e.key === 'Enter' && navigate('dashboard')} tabindex="0" role="button">
    <i class="fa-solid fa-hammer logo-icon"></i>
    <span class="logo-text">TaskForge</span>
  </div>

  <nav class="nav">
    {#each items as { id, label, icon }}
      <button
        class="nav-item"
        class:active={$page === id}
        on:click={() => navigate(id)}
      >
        <span class="nav-icon"><i class="{icon}"></i></span>
        <span>{label}</span>
      </button>
    {/each}
  </nav>

  <div class="sidebar-bottom">
    <div class="stats">
      <div class="stat-row" title="Задачи, ожидающие выполнения">
        <i class="fa-regular fa-circle-down" style="color:#3b82f6;font-size:11px;"></i>
        <span class="stat-label">В очереди</span>
        <span class="stat-val">{stats.stream_len || 0}</span>
      </div>
      <div class="stat-row" title="Задачи, выполняющиеся прямо сейчас">
        <i class="fa-solid fa-gear" style="color:#06b6d4;font-size:11px;"></i>
        <span class="stat-label">Выполняется</span>
        <span class="stat-val">{stats.pending || 0}</span>
      </div>
      <div class="stat-row" title="Состояние обработки очереди">
        <i class="fa-solid fa-power-off" style="color:{stats.paused ? '#ef4444' : '#22c55e'};font-size:11px;"></i>
        <span class="stat-label">Статус</span>
        <span class="stat-val" class:paused={stats.paused}>
          {stats.paused ? 'Пауза' : 'Активна'}
        </span>
      </div>
    </div>

    <div class="key-section">
      <label class="key-label" title="API-ключ для аутентификации запросов">
        <i class="fa-solid fa-key"></i> API-ключ
      </label>
      <input
        class="key-input"
        placeholder="default"
        value={localStorage.getItem('tf_api_key') || 'default'}
        on:input={(e) => localStorage.setItem('tf_api_key', e.target.value)}
      />
    </div>
  </div>
</aside>

<style>
  .sidebar {
    width: 220px;
    background: #0b1120;
    border-right: 1px solid #1e293b;
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
  }
  .logo {
    padding: 18px 16px;
    display: flex;
    align-items: center;
    gap: 10px;
    cursor: pointer;
    border-bottom: 1px solid #1e293b;
    user-select: none;
  }
  .logo-icon { font-size: 20px; color: #60a5fa; }
  .logo-text { font-weight: 800; font-size: 17px; letter-spacing: -.3px; background: linear-gradient(135deg, #60a5fa, #a78bfa); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
  .nav { flex: 1; padding: 10px 8px; display: flex; flex-direction: column; gap: 2px; }
  .nav-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 9px 12px;
    background: transparent;
    color: #94a3b8;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    text-align: left;
    width: 100%;
    transition: all .15s;
  }
  .nav-item:hover { background: #1e293b; color: #e2e8f0; }
  .nav-item.active { background: #1d4ed8; color: #fff; box-shadow: 0 2px 8px #1d4ed844; }
  .nav-icon { width: 20px; text-align: center; }
  .sidebar-bottom {
    padding: 10px;
    border-top: 1px solid #1e293b;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .stats {
    background: #0f172a;
    border-radius: 8px;
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .stat-row { display: flex; align-items: center; gap: 8px; font-size: 12px; }
  .stat-label { color: #64748b; flex: 1; }
  .stat-val { font-weight: 700; color: #60a5fa; }
  .stat-val.paused { color: #ef4444; }
  .key-section { display: flex; flex-direction: column; gap: 4px; }
  .key-label { font-size: 11px; color: #64748b; font-weight: 600; display: flex; align-items: center; gap: 6px; }
  .key-input {
    font-size: 12px;
    padding: 6px 10px;
    background: #0f172a;
    border: 1px solid #1e293b;
    color: #e2e8f0;
    border-radius: 6px;
    outline: none;
    transition: border .15s;
  }
  .key-input:focus { border-color: #3b82f6; }
</style>
