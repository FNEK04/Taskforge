<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import { navigate, toast } from '../lib/stores.js'
  import StatusBadge from '../components/StatusBadge.svelte'

  let jobs = []
  let filterStatus = ''
  let searchQuery = ''
  let loading = true
  let pageNum = 1
  let perPage = 20

  onMount(() => load())

  async function load() {
    loading = true
    try {
      const params = {}
      if (filterStatus) params.status = filterStatus
      jobs = await api.listJobs(params)
    } catch (_) { jobs = [] }
    loading = false
  }

  function handleFilterChange() {
    pageNum = 1
    load()
  }

  $: filtered = searchQuery
    ? jobs.filter(j => j.id?.toLowerCase().includes(searchQuery.toLowerCase()) || j.type?.toLowerCase().includes(searchQuery.toLowerCase()))
    : jobs

  $: totalPages = Math.max(1, Math.ceil(filtered.length / perPage))
  $: paged = filtered.slice((pageNum - 1) * perPage, pageNum * perPage)

  async function cancelJob(id) {
    if (!confirm('Отменить эту задачу?')) return
    try {
      await api.cancelJob(id)
      toast('Задача отменена')
      load()
    } catch (_) {}
  }

  async function retryJob(id) {
    try {
      await api.retryJob(id)
      toast('Задача отправлена на повтор')
      load()
    } catch (_) {}
  }

  function copyId(id) {
    navigator.clipboard.writeText(id).then(() => toast('ID скопирован')).catch(() => {})
  }
</script>

<div class="jobs-page animate-in">
  <div class="header">
    <div>
      <h1 class="page-title">Задачи</h1>
      <p class="page-subtitle">Управление задачами: просмотр, фильтрация, отмена</p>
    </div>
    <button class="btn-primary" on:click={() => navigate('submit-job')}>
      <i class="fa-solid fa-plus"></i> Создать
    </button>
  </div>

  <div class="toolbar">
    <div class="search-box">
      <i class="fa-solid fa-search" style="color:#64748b;font-size:13px;"></i>
      <input
        placeholder="Поиск по ID или типу задачи..."
        bind:value={searchQuery}
      />
      {#if searchQuery}
        <button class="btn-clear" on:click={() => searchQuery = ''}>
          <i class="fa-solid fa-xmark"></i>
        </button>
      {/if}
    </div>
    <select bind:value={filterStatus} on:change={handleFilterChange}>
      <option value="">Все статусы</option>
      <option value="pending">Ожидает</option>
      <option value="scheduled">Запланирована</option>
      <option value="queued">В очереди</option>
      <option value="running">Выполняется</option>
      <option value="completed">Завершена</option>
      <option value="failed">Ошибка</option>
      <option value="cancelled">Отменена</option>
      <option value="dlq">Брак (DLQ)</option>
    </select>
  </div>

  {#if loading}
    <div class="empty"><i class="fa-solid fa-spinner fa-spin"></i> Загрузка...</div>
  {:else if paged.length === 0}
    <div class="empty"><i class="fa-regular fa-inbox"></i> Задачи не найдены</div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th style="width:70px;">ID</th>
            <th>Тип</th>
            <th>Статус</th>
            <th style="width:50px;">Приор.</th>
            <th style="width:70px;">Попытки</th>
            <th style="width:50px;">DAG</th>
            <th>Создана</th>
            <th style="width:80px;"></th>
          </tr>
        </thead>
        <tbody>
          {#each paged as job}
            <tr on:click={() => navigate('job-detail', { id: job.id })}>
              <td class="mono">
                <button class="id-copy" on:click|stopPropagation={() => copyId(job.id)} title="Копировать ID">
                  {job.id?.slice(0, 8)}
                </button>
              </td>
              <td>
                <span class="type-tag">{job.type}</span>
              </td>
              <td><StatusBadge status={job.status} /></td>
              <td>
                <span class="priority" class:high={job.priority >= 7} class:mid={job.priority >= 3 && job.priority < 7} class:low={job.priority < 3}>
                  {job.priority}
                </span>
              </td>
              <td class="mono text-muted">{job.retry_count}/{job.max_retries}</td>
              <td class="mono text-muted">{job.dag_run_id ? job.dag_run_id.slice(0, 6) : '—'}</td>
              <td class="text-muted" style="font-size:11px;">{new Date(job.created_at).toLocaleDateString('ru', {day:'numeric',month:'short',hour:'2-digit',minute:'2-digit'})}</td>
              <td>
                <div class="row-actions">
                  {#if job.status === 'queued' || job.status === 'running'}
                    <button class="btn-sm btn-ghost" style="color:#ef4444;" on:click|stopPropagation={() => cancelJob(job.id)} title="Отменить">
                      <i class="fa-solid fa-ban"></i>
                    </button>
                  {/if}
                  {#if job.status === 'failed'}
                    <button class="btn-sm btn-ghost" style="color:#f59e0b;" on:click|stopPropagation={() => retryJob(job.id)} title="Повторить">
                      <i class="fa-solid fa-rotate"></i>
                    </button>
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="pagination">
      <span class="page-info">{filtered.length} задач, стр. {pageNum} из {totalPages}</span>
      <div class="page-btns">
        <button class="btn-sm btn-ghost" disabled={pageNum <= 1} on:click={() => pageNum--}>
          <i class="fa-solid fa-chevron-left"></i>
        </button>
        {#each Array(totalPages) as _, i}
          <button class="btn-sm btn-ghost" class:active={pageNum === i + 1} on:click={() => pageNum = i + 1}>
            {i + 1}
          </button>
        {/each}
        <button class="btn-sm btn-ghost" disabled={pageNum >= totalPages} on:click={() => pageNum++}>
          <i class="fa-solid fa-chevron-right"></i>
        </button>
      </div>
    </div>
  {/if}
</div>

<style>
  .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
  .toolbar { display: flex; gap: 12px; margin-bottom: 16px; }
  .search-box {
    display: flex;
    align-items: center;
    gap: 8px;
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 8px;
    padding: 0 12px;
    flex: 1;
    max-width: 400px;
    transition: border .2s;
  }
  .search-box:focus-within { border-color: #3b82f6; box-shadow: 0 0 0 3px #3b82f633; }
  .search-box input { border: none; background: transparent; padding: 9px 0; }
  .btn-clear { background: transparent; padding: 2px 4px; color: #64748b; }
  .id-copy { background: transparent; color: #64748b; padding: 0; font-family: inherit; font-size: 12px; }
  .id-copy:hover { color: #60a5fa; }
  .type-tag { background: #1e3a5f; color: #93c5fd; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 500; }
  .priority { font-weight: 700; }
  .priority.high { color: #ef4444; }
  .priority.mid { color: #f59e0b; }
  .priority.low { color: #64748b; }
  .row-actions { display: flex; gap: 4px; }
  .pagination {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-top: 14px;
    color: #64748b;
    font-size: 12px;
  }
  .page-btns { display: flex; gap: 4px; }
  .page-btns .active { background: #1d4ed8; color: #fff; }
  select { width: auto; min-width: 160px; }
</style>
