<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import { toast, fmtTime } from '../lib/stores.js'

  let dlqs = []
  let loading = true
  let replaying = new Set()

  onMount(() => load())

  async function load() {
    loading = true
    dlqs = await api.listDLQ().catch(() => [])
    loading = false
  }

  async function replay(id) {
    replaying.add(id)
    try {
      await api.replayDLQ(id)
      toast('Задача возвращена в очередь')
      load()
    } catch (_) { toast('Ошибка при возврате', 'error') }
    replaying.delete(id)
  }

  async function replayAll() {
    if (!confirm(`Вернуть все ${dlqs.length} задач из DLQ в очередь?`)) return
    const ds = [...dlqs]
    let ok = 0
    for (const d of ds) {
      try {
        await api.replayDLQ(d.id)
        ok++
      } catch {}
    }
    toast(`Возвращено ${ok} из ${ds.length}`)
    load()
  }

  async function replayFailed() {
    const failed = dlqs.filter(d => d.last_error && d.retry_count < 5)
    if (failed.length === 0) { toast('Нет подходящих задач для возврата'); return }
    if (!confirm(`Вернуть ${failed.length} задач с небольшим числом попыток?`)) return
    let ok = 0
    for (const d of failed) {
      try {
        await api.replayDLQ(d.id)
        ok++
      } catch {}
    }
    toast(`Возвращено ${ok} из ${failed.length}`)
    load()
  }
</script>

<div class="dlq-page animate-in">
  <div class="header">
    <div>
      <h1 class="page-title">Dead Letter Queue (DLQ)</h1>
      <p class="page-subtitle">Задачи, исчерпавшие все попытки выполнения. Можно вернуть в очередь вручную</p>
    </div>
    <div class="header-actions">
      {#if dlqs.length > 0}
        <button class="btn-ghost btn-sm" on:click={replayFailed} title="Вернуть только задачи с малым числом попыток">
          <i class="fa-solid fa-filter"></i> Выборочно
        </button>
        <button class="btn-primary" on:click={replayAll}>
          <i class="fa-solid fa-rotate"></i> Вернуть все
        </button>
      {/if}
    </div>
  </div>

  {#if dlqs.length > 0}
    <div class="dlq-info">
      <i class="fa-solid fa-circle-info" style="color:#3b82f6;"></i>
      <span>Всего бракованных задач: <strong>{dlqs.length}</strong>. Нажмите «Вернуть», чтобы повторно поставить задачу в очередь.</span>
    </div>
  {/if}

  {#if loading}
    <div class="empty"><i class="fa-solid fa-spinner fa-spin"></i> Загрузка...</div>
  {:else if dlqs.length === 0}
    <div class="empty">
      <i class="fa-solid fa-check-circle" style="color:#22c55e;font-size:32px;display:block;margin-bottom:10px;"></i>
      DLQ пуста — бракованные задачи отсутствуют
    </div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID задачи</th>
            <th>Тип</th>
            <th>Причина</th>
            <th>Попыток</th>
            <th>Дата</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each dlqs as d}
            <tr>
              <td class="mono">{d.job_id?.slice(0, 12) || d.id?.slice(0, 12)}</td>
              <td>{d.type || '—'}</td>
              <td class="error-cell" title={d.last_error || ''}>{d.last_error || '—'}</td>
              <td class="mono text-muted">{d.retry_count || 0}</td>
              <td class="text-muted" style="font-size:11px;">{fmtTime(d.created_at) || fmtTime(d.failed_at)}</td>
              <td>
                <button class="btn-sm btn-ghost" style="color:#22c55e;" on:click={() => replay(d.id)} disabled={replaying.has(d.id)}>
                  {#if replaying.has(d.id)}
                    <i class="fa-solid fa-spinner fa-spin"></i>
                  {:else}
                    <i class="fa-solid fa-rotate"></i>
                  {/if}
                  Вернуть
                </button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 12px; }
  .header-actions { display: flex; gap: 8px; }
  .dlq-info {
    background: #0f172a;
    border: 1px solid #1e293b;
    border-radius: 8px;
    padding: 10px 14px;
    margin-bottom: 16px;
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 12px;
    color: #94a3b8;
  }
  .table-wrap { background: #1e293b; border-radius: 12px; border: 1px solid #334155; overflow: hidden; }
  .error-cell { font-size: 12px; color: #f87171; max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
