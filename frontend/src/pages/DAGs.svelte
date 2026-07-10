<script>
  import { onMount } from 'svelte'
  import { api } from '../lib/api.js'
  import { navigate, toast } from '../lib/stores.js'

  let dags = []
  let loading = true
  let template = 'simple'
  let dagInput = ''
  let dagError = ''
  let executing = false
  let execResult = null
  let showForm = false
  let previewDag = null

  const templates = {
    simple: JSON.stringify({
      nodes: [
        { id: "fetch", type: "http-fetch", dependencies: [] },
        { id: "parse", type: "parse-json", dependencies: ["fetch"] },
        { id: "save", type: "db-save", dependencies: ["parse"] }
      ]
    }, null, 2),
    parallel: JSON.stringify({
      nodes: [
        { id: "validate", type: "validate", dependencies: [] },
        { id: "enhance", type: "enrich", dependencies: [] },
        { id: "render", type: "generate", dependencies: ["validate", "enhance"] }
      ]
    }, null, 2),
    chained: JSON.stringify({
      nodes: [
        { id: "step-1", type: "transform", dependencies: [] },
        { id: "step-2", type: "transform", dependencies: ["step-1"] },
        { id: "step-3", type: "transform", dependencies: ["step-2"] },
        { id: "step-4", type: "transform", dependencies: ["step-3"] }
      ]
    }, null, 2)
  }

  onMount(() => load())

  async function load() {
    loading = true
    dags = await api.listDAGs().catch(() => [])
    loading = false
  }

  function selectTemplate(t) {
    template = t
    dagInput = templates[t]
    previewDag = null
  }

  function updatePreview() {
    try {
      const parsed = JSON.parse(dagInput)
      previewDag = parsed
    } catch {
      previewDag = null
    }
  }

  async function createDAG() {
    dagError = ''
    let parsed
    try {
      parsed = JSON.parse(dagInput)
    } catch {
      dagError = 'Неверный JSON'
      return
    }
    if (!parsed.nodes || !Array.isArray(parsed.nodes) || parsed.nodes.length === 0) {
      dagError = 'Нужен массив nodes'
      return
    }
    for (const n of parsed.nodes) {
      if (!n.id) { dagError = 'Каждый node должен иметь id'; return }
    }
    try {
      const dag = await api.createDAG(parsed)
      toast('DAG создан: ' + dag.id.slice(0, 12))
      dagInput = ''
      showForm = false
      previewDag = null
      load()
    } catch (e) { dagError = e.message || 'Ошибка создания DAG' }
  }

  async function executeDAG(dagId) {
    executing = true
    execResult = null
    try {
      const res = await api.executeDAG(dagId)
      execResult = res
      toast('DAG выполняется: ' + dagId.slice(0, 12))
    } catch (e) { dagError = e.message || 'Ошибка выполнения DAG' }
    executing = false
  }

  $: if (dagInput && showForm) updatePreview()

  $: nodeCount = previewDag?.nodes?.length || 0
  $: edgeCount = previewDag?.nodes?.reduce((sum, n) => sum + (n.dependencies?.length || 0), 0) || 0
</script>

<div class="dags-page animate-in">
  <div class="header">
    <div>
      <h1 class="page-title">DAG-графы</h1>
      <p class="page-subtitle">Графы задач с зависимостями: узлы выполняются после завершения предшественников</p>
    </div>
    <button class="btn-primary" on:click={() => { showForm = !showForm; if (showForm) selectTemplate(template) }}>
      {#if showForm}
        <i class="fa-solid fa-xmark"></i> Закрыть
      {:else}
        <i class="fa-solid fa-plus"></i> Новый DAG
      {/if}
    </button>
  </div>

  {#if showForm}
    <div class="card" style="margin-bottom:20px;">
      <div class="card-title"><i class="fa-solid fa-pen-ruler"></i> Новый граф задач</div>

      <div class="template-select">
        <label class="tmpl-label">Шаблон:</label>
        <div class="tmpl-btns">
          {#each ['simple', 'parallel', 'chained'] as t}
            <button class="btn-sm btn-ghost" class:active={template === t} on:click={() => selectTemplate(t)}>
              <i class="fa-solid fa-{t === 'simple' ? 'arrow-right' : t === 'parallel' ? 'arrow-right-arrow-left' : 'chain'}"></i>
              {t === 'simple' ? 'Последовательный' : t === 'parallel' ? 'Параллельный' : 'Цепочка'}
            </button>
          {/each}
        </div>
      </div>

      {#if nodeCount > 0}
        <div class="dag-preview">
          <div class="preview-bar">
            <span class="preview-stat"><i class="fa-solid fa-circle-nodes"></i> {nodeCount} узлов</span>
            <span class="preview-stat"><i class="fa-solid fa-arrow-right-arrow-left"></i> {edgeCount} связей</span>
          </div>
          <div class="preview-graph">
            {#each previewDag.nodes as node, i}
              <div class="graph-node" style="animation-delay: {i * 0.05}s;">
                <span class="graph-node-id">{node.id}</span>
                <span class="graph-node-type">{node.type}</span>
              </div>
              {#if i < previewDag.nodes.length - 1}
                <div class="graph-edge">
                  <i class="fa-solid fa-arrow-down" style="color:#3b82f6;font-size:10px;"></i>
                </div>
              {/if}
            {/each}
          </div>
        </div>
      {/if}

      <div class="field-group" style="margin-top:14px;">
        <label><i class="fa-solid fa-code"></i> Описание графа (JSON)</label>
        <textarea bind:value={dagInput} rows="10"></textarea>
        <span class="hint">
          Формат: массив <code>nodes</code>, каждый c <code>id</code>, <code>type</code>, <code>dependencies</code>
        </span>
      </div>

      {#if dagError}
        <div class="error-box"><i class="fa-solid fa-circle-exclamation"></i> {dagError}</div>
      {/if}

      <button class="btn-primary" style="margin-top:12px;" on:click={createDAG}>
        <i class="fa-solid fa-floppy-disk"></i> Сохранить DAG
      </button>
    </div>
  {/if}

  {#if execResult}
    <div class="card success-box" style="margin-bottom:20px;">
      <div class="success-inner">
        <i class="fa-regular fa-circle-check" style="color:#4ade80;font-size:18px;"></i>
        <div>
          <div style="font-weight:600;">DAG запущен</div>
          <span class="mono">{execResult.dag_run_id}</span>
          <div style="font-size:12px;color:#94a3b8;margin-top:2px;">Создано задач: {execResult.jobs?.length || 0}</div>
        </div>
      </div>
      <button class="btn-sm btn-ghost" on:click={() => navigate('jobs')}>
        Смотреть задачи <i class="fa-solid fa-arrow-right"></i>
      </button>
    </div>
  {/if}

  {#if loading}
    <div class="empty"><i class="fa-solid fa-spinner fa-spin"></i> Загрузка...</div>
  {:else if dags.length === 0}
    <div class="empty"><i class="fa-regular fa-diagram-project"></i> Нет сохранённых DAG</div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th><i class="fa-solid fa-circle-nodes"></i> Узлов</th>
            <th><i class="fa-solid fa-arrow-right-arrow-left"></i> Связей</th>
            <th>Создан</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {#each dags as dag}
            <tr>
              <td class="mono">{dag.id?.slice(0, 12)}</td>
              <td>{Array.isArray(dag.nodes) ? dag.nodes.length : 0}</td>
              <td>{Array.isArray(dag.edges) ? dag.edges.length : 0}</td>
              <td class="text-muted" style="font-size:11px;">{new Date(dag.created_at).toLocaleString('ru')}</td>
              <td>
                <button class="btn-sm btn-ghost" style="color:#22c55e;" on:click={() => executeDAG(dag.id)} disabled={executing}>
                  {#if executing}
                    <i class="fa-solid fa-spinner fa-spin"></i>
                  {:else}
                    <i class="fa-solid fa-play"></i>
                  {/if}
                  Выполнить
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
  .template-select { display: flex; align-items: center; gap: 10px; margin-bottom: 14px; }
  .tmpl-label { font-size: 12px; color: #94a3b8; font-weight: 600; }
  .tmpl-btns { display: flex; gap: 6px; }
  .tmpl-btns .active { background: #1d4ed8; color: #fff; }
  .dag-preview {
    background: #0f172a;
    border-radius: 8px;
    padding: 14px;
    margin-bottom: 14px;
  }
  .preview-bar { display: flex; gap: 16px; margin-bottom: 10px; font-size: 12px; color: #64748b; }
  .preview-stat { display: flex; align-items: center; gap: 5px; }
  .preview-graph {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 6px;
  }
  .graph-node {
    background: #1e293b;
    border: 1px solid #334155;
    border-radius: 8px;
    padding: 6px 16px;
    display: flex;
    align-items: center;
    gap: 8px;
    animation: fadeInUp .3s ease both;
  }
  .graph-node-id { font-weight: 600; font-size: 13px; }
  .graph-node-type { color: #64748b; font-size: 11px; }
  .graph-edge { margin: -2px 0; }
  .field-group { display: flex; flex-direction: column; gap: 4px; margin-bottom: 10px; }
  .field-group label { font-size: 12px; color: #94a3b8; font-weight: 600; display: flex; align-items: center; gap: 6px; }
  .hint { font-size: 11px; color: #475569; }
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
  .success-box { border-left: 3px solid #22c55e; }
  .success-inner { display: flex; align-items: center; gap: 12px; margin-bottom: 10px; }
  .table-wrap { background: #1e293b; border-radius: 12px; border: 1px solid #334155; overflow: hidden; }
  code { background: #1e293b; padding: 1px 6px; border-radius: 4px; }
  @keyframes fadeInUp { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }
</style>
