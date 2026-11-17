<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { MemoryLibrary } from '../api'

const props = defineProps<{
  open: boolean
  libraries: MemoryLibrary[]
  enabledIds: string[]
  loading?: boolean
  saving?: boolean
  error?: string
  loadFailed?: boolean
}>()
const emit = defineEmits<{ confirm: [ids: string[]]; cancel: []; retry: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const selectedIDs = ref<string[]>([])

watch(() => [props.open, props.loading] as const, async ([open, loading]) => {
  if (open) {
    selectedIDs.value = loading ? [] : props.enabledIds.filter((id) => props.libraries.some((library) => library.id === id))
    await nextTick()
    if (props.open && dialog.value && !dialog.value.open) dialog.value.showModal()
  } else if (dialog.value?.open) dialog.value.close()
}, { immediate: true })

function cancel() { if (!props.saving) emit('cancel') }
function confirm() {
  if (!props.loading && !props.saving && !props.loadFailed) emit('confirm', [...selectedIDs.value])
}
</script>

<template>
  <dialog ref="dialog" class="picker" aria-labelledby="memory-picker-title" @click.self="cancel" @cancel.prevent="cancel">
    <form class="picker-body" @submit.prevent="confirm">
      <header class="picker-head">
        <div>
          <h2 id="memory-picker-title">记忆设置</h2>
          <p class="sub">勾选当前对话使用的记忆库，不勾选即禁用</p>
        </div>
        <span class="count">{{ selectedIDs.length }} / {{ libraries.length }}</span>
      </header>
      <div class="picker-tools">
        <button type="button" class="btn btn-ghost btn-sm" :disabled="loading || saving" @click="selectedIDs = libraries.map((library) => library.id)">全选</button>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="loading || saving" @click="selectedIDs = []">全部禁用</button>
      </div>
      <div class="picker-list" :aria-busy="loading">
        <p v-if="loading" class="empty">正在加载记忆库…</p>
        <template v-else>
          <label v-for="library in libraries" :key="library.id" class="item">
            <input v-model="selectedIDs" type="checkbox" :value="library.id" :disabled="saving" />
            <span class="item-content">
              <span class="item-name">{{ library.name }}</span>
              <span v-if="library.description" class="item-description">{{ library.description }}</span>
            </span>
            <span class="scope">{{ library.scope === 'global' ? '全局' : '工作区' }}</span>
          </label>
          <p v-if="!libraries.length" class="empty">暂无可用记忆库</p>
        </template>
      </div>
      <p class="note">全局记忆库可启用；写入需要完全访问权限。</p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <button v-if="loadFailed" type="button" class="btn btn-ghost" @click="emit('retry')">重新加载</button>
      <footer class="picker-foot">
        <button type="button" class="btn btn-ghost" :disabled="saving" @click="cancel">取消</button>
        <button type="submit" class="btn btn-primary" :disabled="loading || saving || loadFailed">{{ saving ? '正在保存…' : '保存设置' }}</button>
      </footer>
    </form>
  </dialog>
</template>

<style scoped>
.picker { width: min(540px, calc(100vw - 40px)); max-height: 82vh; padding: 0; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%); }
.picker::backdrop { background: rgb(22 29 50 / 42%); }
.picker-body { display: flex; flex-direction: column; max-height: 82vh; }
.picker-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 20px 22px 10px; }
.picker-head h2 { font-size: 16px; font-weight: 600; }
.sub { margin-top: 3px; color: var(--muted); font-size: 12.5px; }
.count { flex-shrink: 0; padding: 3px 9px; border-radius: 999px; background: var(--tonal); color: var(--accent); font-size: 11.5px; }
.picker-tools { display: flex; gap: 6px; padding: 0 22px 8px; }
.picker-list { display: flex; flex-direction: column; gap: 1px; min-height: 120px; padding: 4px 16px; overflow-y: auto; }
.item { display: flex; align-items: center; gap: 9px; padding: 10px 8px; border-radius: 8px; cursor: pointer; }
.item:hover { background: var(--tonal); }
.item input { flex-shrink: 0; width: 15px; height: 15px; accent-color: var(--accent); }
.item-content { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.item-name { font-size: 13px; overflow-wrap: anywhere; }
.item-description { color: var(--muted); font-size: 12px; overflow-wrap: anywhere; }
.scope { flex-shrink: 0; font-size: 11px; color: var(--muted); }
.empty { padding: 16px 8px; color: var(--muted); font-size: 12.5px; text-align: center; }
.note, .error { padding: 0 22px; font-size: 12px; line-height: 1.6; }
.note { color: var(--muted); margin: 12px 0 0; }
.error { color: var(--danger, #b42318); margin-top: 8px; }
.picker-foot { display: flex; justify-content: flex-end; gap: 8px; padding: 12px 22px 18px; }
</style>
