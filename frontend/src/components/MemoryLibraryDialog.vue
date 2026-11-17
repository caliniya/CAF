<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import type { MemoryLibrary } from '../api'

const props = defineProps<{
  open: boolean
  mode: 'create' | 'rename'
  initialName?: string
  workspaceId?: string
  workspaceName?: string
  busy: boolean
  error: string
}>()
const emit = defineEmits<{ confirm: [name: string, scope: MemoryLibrary['scope']]; cancel: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const name = ref('')
const scope = ref<MemoryLibrary['scope']>('global')
const titleId = useId()

function cancel() { if (!props.busy) emit('cancel') }
function confirm() {
  if (!props.busy && name.value.trim() && (scope.value !== 'workspace' || props.workspaceId || props.mode === 'rename')) {
    emit('confirm', name.value.trim(), scope.value)
  }
}
watch(() => props.open, async (open) => {
  if (open) {
    name.value = props.initialName ?? ''
    scope.value = props.workspaceId ? 'workspace' : 'global'
  }
  await nextTick()
  if (props.open !== open) return
  if (open && dialog.value && !dialog.value.open) dialog.value.showModal()
  else if (!open && dialog.value?.open) dialog.value.close()
}, { immediate: true })
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="memory-library-dialog" :aria-labelledby="titleId" :aria-busy="busy"
      @click.self="cancel" @cancel.prevent="cancel" @keydown.stop>
      <form @submit.prevent="confirm">
        <h2 :id="titleId">{{ mode === 'create' ? '创建记忆' : '重命名记忆' }}</h2>
        <label class="field">
          <span>名称</span>
          <input v-model="name" type="text" required autofocus :disabled="busy" />
        </label>
        <label v-if="mode === 'create'" class="field">
          <span>作用范围</span>
          <select v-model="scope" :disabled="busy">
            <option value="global">全局记忆</option>
            <option value="workspace" :disabled="!workspaceId">当前对话工作区{{ workspaceName ? ` · ${workspaceName}` : '' }}</option>
          </select>
        </label>
        <p v-if="mode === 'create'" class="note">{{ scope === 'global' ? '所有工作区都可以看见此记忆。' : '只有当前工作区可以看见此记忆。' }}创建后可在每个对话中单独启用。</p>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <footer>
          <button type="button" class="btn btn-ghost" :disabled="busy" @click="cancel">取消</button>
          <button type="submit" class="btn btn-primary" :disabled="busy || !name.trim()">{{ busy ? '处理中…' : mode === 'create' ? '创建' : '保存名称' }}</button>
        </footer>
      </form>
    </dialog>
  </Teleport>
</template>

<style scoped>
.memory-library-dialog { width: min(420px, calc(100vw - 40px)); max-height: calc(100vh - 40px); padding: 20px; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%); }
.memory-library-dialog::backdrop { background: rgb(22 29 50 / 42%); }
h2 { font-size: 16px; font-weight: 600; margin-bottom: 18px; }
.field { display: flex; flex-direction: column; gap: 6px; margin-top: 14px; font-size: 13px; }
.field > span, .note { color: var(--muted); }
input, select { width: 100%; height: 36px; padding: 0 10px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); color: var(--ink); font-size: 13px; }
.note, .error { margin-top: 12px; font-size: 12px; line-height: 1.6; }
.error { color: var(--danger); }
footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
</style>
