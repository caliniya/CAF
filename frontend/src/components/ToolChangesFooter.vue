<script setup lang="ts">
import { computed } from 'vue'
import { File, Redo2, Undo2 } from '@lucide/vue'
import type { ToolOperation } from '../api'

const props = defineProps<{
  operations: ToolOperation[]
  canUndo?: boolean
  canRedo?: boolean
  busy?: boolean
}>()
const emit = defineEmits<{ change: [undone: boolean] }>()
const undone = computed(() => props.operations.every((operation) => Boolean(operation.state)))
const discarded = computed(() => props.operations.some((operation) => operation.state === 'discarded'))
const rows = computed(() => props.operations.flatMap((operation) => (operation.changes ?? []).map((change, index) => {
  const target = change.target.replaceAll('\\', '/')
  const workspace = operation.workspace?.replaceAll('\\', '/').replace(/\/$/, '')
  const label = workspace && target.toLowerCase().startsWith(workspace.toLowerCase() + '/')
    ? target.slice(workspace.length + 1) : target
  return { ...change, label, key: `${operation.id}:${index}` }
})))
const actionNames = { create: '创建', modify: '修改', delete: '删除' }
</script>

<template>
  <section class="tool-changes" :class="{ undone }" aria-label="本轮工具改动">
    <header class="changes-header">
      <span class="changes-heading">本轮改动 <small>{{ operations.length }} 次操作<span v-if="undone"> · 已撤销</span></small></span>
      <button v-if="!undone" type="button" class="changes-action" :disabled="busy || !canUndo"
        :title="canUndo ? '撤销本轮工具产生的改动' : '请先撤销后续回复的改动'" @click="emit('change', true)">
        <Undo2 :size="14" />撤销改动
      </button>
      <button v-else-if="!discarded" type="button" class="changes-action" :disabled="busy || !canRedo"
        :title="canRedo ? '恢复本轮工具完成后的状态' : '请先取消此前回复的撤销'" @click="emit('change', false)">
        <Redo2 :size="14" />取消撤销
      </button>
    </header>
    <ul v-if="rows.length" class="changes-list">
      <li v-for="row in rows" :key="row.key" class="change-row">
        <File :size="15" class="change-icon" aria-hidden="true" />
        <span class="change-kind">{{ actionNames[row.action] }}</span>
        <span class="change-target" :title="row.target">{{ row.label }}</span>
        <span v-if="row.binary" class="change-bytes">{{ row.beforeBytes.toLocaleString() }} → {{ row.afterBytes.toLocaleString() }} 字节</span>
        <span v-else class="change-lines" :title="`${row.beforeBytes.toLocaleString()} → ${row.afterBytes.toLocaleString()} 字节`">
          <span v-if="row.approximate" class="change-estimate">约</span>
          <span class="lines-added">+{{ row.addedLines ?? 0 }}</span>
          <span class="lines-removed">−{{ row.removedLines ?? 0 }}</span><span class="change-unit">行</span>
        </span>
      </li>
    </ul>
    <p v-if="discarded" class="changes-note">撤销后已产生新的改动，原改动保持撤销。</p>
  </section>
</template>

<style scoped>
.tool-changes {
  margin-top: 18px;
  border: 1px solid var(--outline);
  border-radius: 10px;
  overflow: hidden;
  font-size: 12px;
}
.changes-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 12px;
}
.changes-heading { font-weight: 500; }
.changes-heading small { margin-left: 5px; color: var(--muted); font-size: 11px; font-weight: 400; }
.changes-action {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
  padding: 4px 7px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--ink);
  font: inherit;
  cursor: pointer;
}
.changes-action:hover:not(:disabled) { background: var(--tonal); }
.changes-action:disabled { opacity: .4; cursor: default; }
.changes-action:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.changes-list { margin: 0; padding: 0; list-style: none; }
.change-row { display: flex; align-items: center; gap: 7px; padding: 8px 12px; border-top: 1px solid var(--outline); }
.change-icon, .change-kind, .change-unit, .change-estimate { flex-shrink: 0; color: var(--muted); }
.change-kind { font-size: 11px; }
.change-target { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.change-lines, .change-bytes { display: inline-flex; gap: 6px; flex-shrink: 0; font-size: 11px; font-variant-numeric: tabular-nums; }
.lines-added { color: var(--success); }
.lines-removed { color: var(--danger); }
.change-bytes { color: var(--muted); }
.undone .change-row { opacity: .6; }
.changes-note { margin: 0; padding: 0 12px 9px; color: var(--muted); font-size: 11px; }
@media (max-width: 480px) {
  .changes-heading small { display: block; margin: 2px 0 0; }
  .change-row { flex-wrap: wrap; }
  .change-target { flex-basis: calc(100% - 58px); }
  .change-lines, .change-bytes { margin-left: 22px; }
}
</style>
