<script>
  import { STATUS_COLORS, STATUS_LABELS, STATUS_ICONS } from '../lib/stores.js'
  export let status = 'pending'
  $: color = STATUS_COLORS[status] || '#6b7280'
  $: label = STATUS_LABELS[status] || status
  $: icon = STATUS_ICONS[status] || 'fa-regular fa-question'
</script>

<span class="badge" style="--bg: {color}1a; --color: {color}; --border: {color}33;">
  <i class="{icon}" style="font-size: 11px;"></i>
  {label}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 3px 10px;
    border-radius: 20px;
    font-size: 12px;
    font-weight: 600;
    background: var(--bg);
    color: var(--color);
    border: 1px solid var(--border);
    white-space: nowrap;
  }
</style>
