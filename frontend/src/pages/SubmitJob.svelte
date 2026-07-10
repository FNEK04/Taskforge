<script>
  import { api } from '../lib/api.js'
  import { navigate, toast } from '../lib/stores.js'

  let type = 'default'
  let payload = '{\n  "task": "hello"\n}'
  let priority = 5
  let maxRetries = 3
  let scheduledAt = ''
  let idempotencyKey = ''
  let submitting = false
  let error = ''
  let result = null

  async function submit() {
    submitting = true
    error = ''
    result = null
    try {
      let parsed
      try { parsed = JSON.parse(payload) } catch { parsed = payload }
      const body = {
        type: type || 'default',
        payload: parsed,
        priority: parseInt(priority) || 0,
        max_retries: parseInt(maxRetries) || 3,
      }
      if (scheduledAt) body.scheduled_at = new Date(scheduledAt).toISOString()
      if (idempotencyKey) body.idempotency_key = idempotencyKey
      result = await api.submitJob(body)
      toast('Задача создана: ' + result.id.slice(0, 12))
      payload = '{\n  "task": "hello"\n}'
      type = 'default'
      priority = 5
      maxRetries = 3
      scheduledAt = ''
      idempotencyKey = ''
    } catch (e) { error = e.message || 'Ошибка при создании' }
    submitting = false
  }
</script>

<div class="submit-page animate-in">
  <div class="breadcrumb">
    <button class="link" on:click={() => navigate('jobs')}>
      <i class="fa-solid fa-arrow-left"></i> Все задачи
    </button>
    <span>/ Создать</span>
  </div>

  <h1 class="page-title">Создать задачу</h1>
  <p class="page-subtitle">Добавьте новую задачу в очередь на выполнение</p>

  {#if result}
    <div class="success-box">
      <i class="fa-regular fa-circle-check" style="font-size:18px;"></i>
      <div>
        <div style="font-weight:600;margin-bottom:2px;">Задача создана</div>
        <span class="mono">{result.id}</span>
      </div>
      <button class="btn-sm btn-ghost" on:click={() => navigate('job-detail', { id: result.id })}>
        Подробнее <i class="fa-solid fa-arrow-right" style="font-size:10px;"></i>
      </button>
    </div>
  {/if}

  {#if error}
    <div class="error-box"><i class="fa-solid fa-circle-exclamation"></i> {error}</div>
  {/if}

  <div class="card">
    <form on:submit|preventDefault={submit}>
      <div class="field-row">
        <div class="field-group">
          <label><i class="fa-solid fa-tag"></i> Тип задачи</label>
          <input bind:value={type} placeholder="default" />
          <span class="hint">Произвольное название для группировки задач одного типа</span>
        </div>
        <div class="field-group">
          <label><i class="fa-solid fa-arrow-up-wide-short"></i> Приоритет</label>
          <input type="number" bind:value={priority} min="0" max="10" />
          <span class="hint">0 — низкий, 10 — максимальный. Чем выше, тем раньше выполнится</span>
        </div>
        <div class="field-group">
          <label><i class="fa-solid fa-rotate"></i> Макс. повторов</label>
          <input type="number" bind:value={maxRetries} min="0" max="20" />
          <span class="hint">Сколько раз повторять при ошибке. 0 — без повторов</span>
        </div>
        <div class="field-group">
          <label><i class="fa-solid fa-fingerprint"></i> Ключ идемпотентности</label>
          <input bind:value={idempotencyKey} placeholder="напр. order-123" />
          <span class="hint">Повторная отправка с тем же ключом игнорируется</span>
        </div>
      </div>

      <div class="field-group" style="margin-top:14px;">
        <label><i class="fa-solid fa-cube"></i> Полезная нагрузка (JSON)</label>
        <textarea bind:value={payload} rows="8"></textarea>
        <span class="hint">JSON-объект с данными, которые будут переданы обработчику задачи</span>
      </div>

      <div class="field-group" style="margin-top:14px;">
        <label><i class="fa-regular fa-clock"></i> Отложенный запуск (опционально)</label>
        <input type="datetime-local" bind:value={scheduledAt} />
        <span class="hint">Оставьте пустым для немедленного выполнения задачи</span>
      </div>

      <div class="submit-hints">
        <div class="hint-box">
          <i class="fa-solid fa-lightbulb" style="color:#f59e0b;"></i>
          <span>Пример payload: <code>{`{ "task": "send-email", "to": "user@example.com" }`}</code></span>
        </div>
      </div>

      <button class="btn-primary" style="margin-top:18px;padding:11px 24px;width:100%;font-size:14px;" type="submit" disabled={submitting}>
        {#if submitting}
          <i class="fa-solid fa-spinner fa-spin"></i> Создание...
        {:else}
          <i class="fa-solid fa-cloud-arrow-up"></i> Создать задачу
        {/if}
      </button>
    </form>
  </div>
</div>

<style>
  .submit-page { max-width: 640px; }
  .field-row { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  .field-group { display: flex; flex-direction: column; gap: 4px; }
  .field-group label { font-size: 12px; color: #94a3b8; font-weight: 600; display: flex; align-items: center; gap: 6px; }
  .success-box {
    background: #14532d;
    border: 1px solid #22c55e44;
    border-radius: 10px;
    padding: 14px;
    margin-bottom: 14px;
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .error-box {
    background: #450a0a;
    border: 1px solid #ef444444;
    border-radius: 10px;
    padding: 12px 14px;
    margin-bottom: 14px;
    color: #f87171;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .submit-hints { margin-top: 14px; }
  .hint-box {
    background: #0f172a;
    border-radius: 8px;
    padding: 10px 12px;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: #94a3b8;
  }
  code { background: #1e293b; padding: 1px 6px; border-radius: 4px; font-size: 11px; }
</style>
