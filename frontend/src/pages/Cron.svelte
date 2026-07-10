<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import { toast, fmtTime } from '../lib/stores.js'

  let crons = []
  let loading = true
  let showForm = false
  let cronType = 'default'
  let cronPayload = '{}'
  let cronExpr = '*/5 * * * *'
  let cronError = ''

  onMount(() => load())

  async function load() {
    loading = true
    crons = await api.listCron().catch(() => [])
    loading = false
  }

  async function createCron() {
    cronError = ''
    let parsed
    try { parsed = JSON.parse(cronPayload) } catch { parsed = cronPayload }
    try {
      await api.createCron({
        type: cronType || 'default',
        payload: parsed,
        cron_expr: cronExpr,
      })
      toast('Расписание создано')
      showForm = false
      cronType = 'default'
      cronPayload = '{}'
      cronExpr = '*/5 * * * *'
      load()
    } catch (e) { cronError = e.message || 'Ошибка' }
  }

  async function deleteCron(id) {
    if (!confirm('Удалить расписание?')) return
    try {
      await api.deleteCron(id)
      toast('Расписание удалено')
      load()
    } catch (_) {}
  }

  function describeCron(expr) {
    const parts = expr.split(/\s+/)
    if (parts.length < 5) return expr
    const [min, hour, day, month, weekday] = parts
    let desc = 'Каждую '
    if (min === '*' && hour === '*') desc += 'минуту'
    else if (min === '*' && hour !== '*') desc += `минуту с ${hour}:00 до ${hour}:59`
    else if (min !== '*' && hour === '*') desc += `в :${min} каждого часа`
    else desc += `в ${hour}:${min}`
    if (day !== '*') desc += `, ${day}-го числа`
    if (month !== '*') desc += `, ${month}-го месяца`
    if (weekday !== '*') desc += `, день недели ${weekday}`
    return desc
  }
</script>

<div class="cron-page animate-in">
  <div class="header">
    <div>
      <h1 class="page-title">Планировщик (Cron)</h1>
      <p class="page-subtitle">Автоматически создавайте задачи по расписанию</p>
    </div>
    <button class="btn-primary" on:click={() => showForm = !showForm}>
      {#if showForm}
        <i class="fa-solid fa-xmark"></i> Закрыть
      {:else}
        <i class="fa-solid fa-plus"></i> Новое расписание
      {/if}
    </button>
  </div>

  {#if showForm}
    <div class="card" style="margin-bottom:20px;">
      <div class="card-title"><i class="fa-regular fa-clock"></i> Новое cron-расписание</div>

      <div class="field-row">
        <div class="field-group">
          <label><i class="fa-solid fa-tag"></i> Тип задачи</label>
          <input bind:value={cronType} placeholder="default" />
          <span class="hint">Тип создаваемых задач</span>
        </div>
        <div class="field-group">
          <label><i class="fa-regular fa-clock"></i> Cron-выражение</label>
          <input bind:value={cronExpr} placeholder="*/5 * * * *" />
          <span class="hint">Формат: минуты часы день месяц день_недели</span>
        </div>
      </div>

      {#if cronExpr}
        <div class="cron-preview">
          <i class="fa-regular fa-circle-info" style="color:#3b82f6;"></i>
          <span>{describeCron(cronExpr)}</span>
        </div>
      {/if}

      <div class="field-group" style="margin-top:12px;">
        <label><i class="fa-solid fa-cube"></i> Payload (JSON)</label>
        <textarea bind:value={cronPayload} rows="4"></textarea>
        <span class="hint">JSON, который будет передан каждой созданной задаче</span>
      </div>

      <div class="examples">
        <span class="hint" style="margin-bottom:4px;">Примеры:</span>
        <div class="example-chips">
          <button class="chip" on:click={() => cronExpr = '*/5 * * * *'}>
            <i class="fa-regular fa-clock"></i> Каждые 5 мин
          </button>
          <button class="chip" on:click={() => cronExpr = '0 * * * *'}>
            <i class="fa-regular fa-clock"></i> Каждый час
          </button>
          <button class="chip" on:click={() => cronExpr = '0 9 * * 1-5'}>
            <i class="fa-regular fa-calendar"></i> Будни в 9:00
          </button>
          <button class="chip" on:click={() => cronExpr = '0 0 * * *'}>
            <i class="fa-regular fa-moon"></i> Каждый день в полночь
          </button>
        </div>
      </div>

      {#if cronError}
        <div class="error-box"><i class="fa-solid fa-circle-exclamation"></i> {cronError}</div>
      {/if}

      <button class="btn-primary" style="margin-top:12px;" on:click={createCron}>
        <i class="fa-solid fa-floppy-disk"></i> Сохранить
      </button>
    </div>
  {/if}

  {#if loading}
    <div class="empty"><i class="fa-solid fa-spinner fa-spin"></i> Загрузка...</div>
  {:else if crons.length === 0}
    <div class="empty"><i class="fa-regular fa-calendar"></i> Нет расписаний</div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Тип</th>
            <th>Выражение</th>
            <th>Описание</th>
            <th>Статус</th>
            <th>Создан</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each crons as cron}
            <tr>
              <td class="mono">{cron.id?.slice(0, 10)}</td>
              <td>{cron.type}</td>
              <td><code>{cron.cron_expression}</code></td>
              <td class="text-muted" style="font-size:11px;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">
                {describeCron(cron.cron_expression)}
              </td>
              <td>
                <span class="badge" class:enabled={cron.enabled !== false} class:disabled={cron.enabled === false}>
                  <i class="fa-{cron.enabled !== false ? 'regular fa-circle-check' : 'regular fa-circle-xmark'}"></i>
                  {cron.enabled !== false ? 'Активно' : 'Отключено'}
                </span>
              </td>
              <td class="text-muted" style="font-size:11px;">{fmtTime(cron.created_at)}</td>
              <td>
                <button class="btn-sm btn-ghost" style="color:#ef4444;" on:click={() => deleteCron(cron.id)} title="Удалить расписание">
                  <i class="fa-solid fa-trash-can"></i>
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
  .header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; }
  .field-row { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  .field-group { display: flex; flex-direction: column; gap: 4px; }
  .field-group label { font-size: 12px; color: #94a3b8; font-weight: 600; display: flex; align-items: center; gap: 6px; }
  .hint { font-size: 11px; color: #475569; }
  .cron-preview {
    background: #0f172a;
    border-radius: 6px;
    padding: 8px 12px;
    margin-top: 8px;
    font-size: 12px;
    color: #94a3b8;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .examples { margin-top: 10px; }
  .example-chips { display: flex; gap: 6px; flex-wrap: wrap; }
  .chip {
    background: #0f172a;
    border: 1px solid #1e293b;
    border-radius: 6px;
    padding: 4px 10px;
    font-size: 11px;
    color: #94a3b8;
    display: flex;
    align-items: center;
    gap: 5px;
    cursor: pointer;
    transition: border .15s;
  }
  .chip:hover { border-color: #3b82f6; color: #e2e8f0; }
  .error-box {
    background: #450a0a;
    border: 1px solid #ef444444;
    border-radius: 8px;
    padding: 10px;
    margin-bottom: 10px;
    color: #f87171;
    font-size: 13px;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .table-wrap { background: #1e293b; border-radius: 12px; border: 1px solid #334155; overflow: hidden; }
  .badge { padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: 600; display: inline-flex; align-items: center; gap: 5px; }
  .badge.enabled { background: #14532d; color: #4ade80; }
  .badge.disabled { background: #451a03; color: #fb923c; }
  code { background: #0f172a; padding: 2px 8px; border-radius: 4px; font-size: 12px; }
</style>
