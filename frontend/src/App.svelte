<script>
  import { page, pageParams, toasts, liveEvents } from './lib/stores.js'
  import Sidebar from './components/Sidebar.svelte'
  import Dashboard from './pages/Dashboard.svelte'
  import Jobs from './pages/Jobs.svelte'
  import JobDetail from './pages/JobDetail.svelte'
  import SubmitJob from './pages/SubmitJob.svelte'
  import DAGs from './pages/DAGs.svelte'
  import Cron from './pages/Cron.svelte'
  import DLQ from './pages/DLQ.svelte'

  function connectWS() {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const apiKey = localStorage.getItem('tf_api_key') || 'default'
    const url = `${protocol}//${location.host}/api/v1/ws?api_key=${apiKey}`

    function connect() {
      const ws = new WebSocket(url)
      ws.onopen = () => {}
      ws.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data)
          liveEvents.update(arr => [{ time: Date.now(), ...msg }, ...arr].slice(0, 200))
        } catch (_) {}
      }
      ws.onclose = () => setTimeout(connect, 5000)
      ws.onerror = () => ws?.close()
    }
    connect()
  }

  connectWS()
</script>

<div class="app">
  <Sidebar />
  <main class="main">
    <div class="page animate-in">
      {#if $page === 'dashboard'}
        <Dashboard />
      {:else if $page === 'jobs'}
        <Jobs />
      {:else if $page === 'job-detail'}
        <JobDetail id={$pageParams.id} />
      {:else if $page === 'submit-job'}
        <SubmitJob />
      {:else if $page === 'dags'}
        <DAGs />
      {:else if $page === 'cron'}
        <Cron />
      {:else if $page === 'dlq'}
        <DLQ />
      {/if}
    </div>
  </main>
</div>

{#each $toasts as t (t.id)}
  <div class="toast toast-{t.type} animate-slide-up">
    <i class="fa-{t.type === 'success' ? 'regular fa-circle-check' : 'regular fa-circle-xmark'}"></i>
    {t.msg}
  </div>
{/each}

<style>
  :global(*) { margin: 0; padding: 0; box-sizing: border-box; }
  :global(body) {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    background: #0f172a;
    color: #e2e8f0;
    font-size: 14px;
    line-height: 1.5;
    -webkit-font-smoothing: antialiased;
  }
  :global(a) { color: #60a5fa; text-decoration: none; }
  :global(button) {
    cursor: pointer;
    border: none;
    padding: 8px 16px;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    font-family: inherit;
    transition: all .2s;
  }
  :global(button:active) { transform: scale(.96); }
  :global(button:disabled) { opacity: .5; cursor: not-allowed; }
  :global(input), :global(textarea), :global(select) {
    font-family: inherit;
    background: #1e293b;
    border: 1px solid #334155;
    color: #e2e8f0;
    padding: 9px 12px;
    border-radius: 8px;
    font-size: 13px;
    width: 100%;
    transition: border .2s, box-shadow .2s;
    outline: none;
  }
  :global(input:focus), :global(textarea:focus), :global(select:focus) {
    border-color: #3b82f6;
    box-shadow: 0 0 0 3px #3b82f633;
  }
  :global(textarea) { font-family: 'JetBrains Mono', 'Fira Code', monospace; resize: vertical; min-height: 80px; font-size: 12px; }
  :global(table) { width: 100%; border-collapse: seperate; border-spacing: 0; }
  :global(th) {
    padding: 10px 14px;
    text-align: left;
    font-weight: 600;
    color: #64748b;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: .06em;
    border-bottom: 1px solid #1e293b;
    white-space: nowrap;
  }
  :global(td) {
    padding: 10px 14px;
    border-bottom: 1px solid #1e293b;
    font-size: 13px;
  }
  :global(tr:last-child td) { border-bottom: none; }
  :global(tr:hover td) { background: #1a2332; }

  .app { display: flex; height: 100vh; overflow: hidden; }
  .main { flex: 1; overflow-y: auto; }
  .page { padding: 28px 32px; max-width: 1200px; }

  .animate-in { animation: fadeInUp .35s ease; }
  @keyframes fadeInUp { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: translateY(0); } }

  .toast {
    position: fixed;
    bottom: 24px;
    right: 24px;
    padding: 12px 20px;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 10px;
    z-index: 1000;
    box-shadow: 0 8px 32px rgba(0,0,0,.5);
    backdrop-filter: blur(8px);
  }
  .toast-success { background: #14532dee; color: #4ade80; border: 1px solid #22c55e44; }
  .toast-error { background: #450a0aee; color: #f87171; border: 1px solid #ef444444; }
  .animate-slide-up { animation: slideUp .35s ease; }
  @keyframes slideUp { from { transform: translateY(20px); opacity: 0; } to { transform: translateY(0); opacity: 1; } }

  :global(.page-title) { font-size: 22px; font-weight: 700; margin-bottom: 4px; letter-spacing: -.3px; }
  :global(.page-subtitle) { color: #64748b; font-size: 13px; margin-bottom: 20px; }
  :global(.card) {
    background: #1e293b;
    border-radius: 12px;
    border: 1px solid #334155;
    padding: 20px;
    transition: border-color .2s, box-shadow .2s;
  }
  :global(.card:hover) { border-color: #3b82f633; }
  :global(.card-title) {
    font-size: 14px;
    font-weight: 600;
    color: #94a3b8;
    text-transform: uppercase;
    letter-spacing: .05em;
    margin-bottom: 16px;
  }
  :global(.btn-primary) { background: linear-gradient(135deg, #2563eb, #1d4ed8); color: #fff; }
  :global(.btn-primary:hover) { background: linear-gradient(135deg, #1d4ed8, #1e40af); box-shadow: 0 4px 16px #2563eb33; }
  :global(.btn-success) { background: linear-gradient(135deg, #16a34a, #15803d); color: #fff; }
  :global(.btn-success:hover) { box-shadow: 0 4px 16px #16a34a33; }
  :global(.btn-danger) { background: linear-gradient(135deg, #dc2626, #b91c1c); color: #fff; }
  :global(.btn-danger:hover) { box-shadow: 0 4px 16px #dc262633; }
  :global(.btn-ghost) { background: transparent; color: #94a3b8; }
  :global(.btn-ghost:hover) { background: #1e293b; color: #e2e8f0; }
  :global(.btn-sm) { padding: 5px 10px; font-size: 12px; border-radius: 6px; }
  :global(.mono) { font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 12px; }
  :global(.text-muted) { color: #64748b; }
  :global(.text-error) { color: #f87171; }
  :global(.text-success) { color: #4ade80; }
  :global(h1) { font-size: 22px; font-weight: 700; margin-bottom: 20px; letter-spacing: -.3px; }
  :global(h2) { font-size: 15px; font-weight: 600; color: #94a3b8; margin-bottom: 14px; text-transform: uppercase; letter-spacing: .05em; }
  :global(.empty) { color: #475569; padding: 32px; text-align: center; font-size: 14px; }
  :global(.breadcrumb) { display: flex; gap: 8px; align-items: center; margin-bottom: 16px; font-size: 13px; color: #64748b; }
  :global(.link) { background: transparent; color: #60a5fa; padding: 0; font-size: 13px; }
  :global(.link:hover) { text-decoration: underline; }
  :global(.field-group) { display: flex; flex-direction: column; gap: 4px; }
  :global(.field-group label) { font-size: 12px; color: #94a3b8; font-weight: 600; text-transform: uppercase; letter-spacing: .05em; }
  :global(.hint) { font-size: 11px; color: #475569; line-height: 1.4; }
  :global(.field-row) { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  :global(.table-wrap) { background: #1e293b; border-radius: 12px; border: 1px solid #334155; overflow: hidden; }
  :global(::-webkit-scrollbar) { width: 6px; }
  :global(::-webkit-scrollbar-track) { background: transparent; }
  :global(::-webkit-scrollbar-thumb) { background: #334155; border-radius: 3px; }
  :global(::-webkit-scrollbar-thumb:hover) { background: #475569; }
</style>
