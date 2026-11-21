<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import type { MemoryEntry, MemoryEntryInput } from '../api'

const props = defineProps<{
  open: boolean
  mode: 'create' | 'edit'
  memoryId?: string
  entry?: MemoryEntry
  busy: boolean
  error: string
}>()
const emit = defineEmits<{ confirm: [input: MemoryEntryInput]; cancel: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const titleId = useId()
const name = ref('')
const date = ref('')
const data = ref('')
const tags = ref('')
const dateValid = computed(() => props.mode === 'create'
  || /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.test(date.value.trim()))
const valid = computed(() => !!name.value.trim() && !!data.value.trim() && dateValid.value)

function cancel() { if (!props.busy) emit('cancel') }
function confirm() {
  if (props.busy || !valid.value) return
  emit('confirm', {
    name: name.value.trim(), data: data.value,
    ...(props.mode === 'edit' ? { date: date.value.trim() } : {}),
    tags: [...new Set(tags.value.split(/[,，\n]/).map(tag => tag.trim()).filter(Boolean))],
  })
}

watch(() => props.open, async (open) => {
  if (open) {
    name.value = props.entry?.name ?? ''
    date.value = props.entry?.date ?? ''
    data.value = props.entry?.data ?? ''
    tags.value = (props.entry?.description ?? []).join('\n')
  }
  await nextTick()
  if (props.open !== open) return
  if (open && dialog.value && !dialog.value.open) dialog.value.showModal()
  else if (!open && dialog.value?.open) dialog.value.close()
}, { immediate: true })
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="memory-entry-dialog" :aria-labelledby="titleId" :aria-busy="busy"
      closedby="closerequest" @cancel.prevent="cancel" @keydown.stop>
      <form @submit.prevent="confirm">
        <h2 :id="titleId">{{ mode === 'create' ? '创建记忆' : '编辑记忆' }}</h2>
        <div v-if="mode === 'edit'" class="entry-id"><span>记忆 ID</span><code>{{ memoryId }}</code></div>
        <label class="field">
          <span>名称</span>
          <input v-model="name" type="text" aria-label="记忆名称" required autofocus :disabled="busy" />
        </label>
        <label v-if="mode === 'edit'" class="field">
          <span>时间</span>
          <input v-model="date" type="text" aria-label="记忆时间" required :disabled="busy" :aria-invalid="!dateValid" />
          <span class="field-help">包含时区的时间，例如 2026-11-18T10:00:00+08:00。</span>
        </label>
        <label class="field">
          <span>内容</span>
          <textarea v-model="data" aria-label="记忆内容" rows="7" required :disabled="busy" />
        </label>
        <label class="field">
          <span>标签</span>
          <textarea v-model="tags" aria-label="记忆标签" rows="3" :disabled="busy" />
          <span class="field-help">每行一个标签，也可用逗号分隔。</span>
        </label>
        <p v-if="error" class="entry-error" role="alert">{{ error }}</p>
        <footer>
          <button type="button" class="btn btn-ghost" :disabled="busy" @click="cancel">取消</button>
          <button type="submit" class="btn btn-primary" :disabled="busy || !valid">{{ busy ? '正在保存…' : mode === 'create' ? '创建' : '确认保存' }}</button>
        </footer>
      </form>
    </dialog>
  </Teleport>
</template>

<style scoped>
.memory-entry-dialog { width: min(580px, calc(100vw - 40px)); max-height: calc(100vh - 40px); padding: 20px; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%); overflow-y: auto; }
.memory-entry-dialog::backdrop { background: rgb(22 29 50 / 42%); }
h2 { font-size: 16px; font-weight: 600; margin-bottom: 16px; }
.entry-id { display: flex; flex-direction: column; gap: 4px; color: var(--muted); font-size: 12px; }
.entry-id code { font-family: var(--font-mono); overflow-wrap: anywhere; }
.field { display: flex; flex-direction: column; gap: 6px; margin-top: 14px; font-size: 13px; }
.field > span { color: var(--muted); }
input, textarea { width: 100%; padding: 8px 10px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); color: var(--ink); font-size: 13px; }
textarea { min-height: 70px; resize: vertical; line-height: 1.7; }
.field .field-help { font-size: 11px; }
.entry-error { margin-top: 12px; color: var(--danger); font-size: 12px; }
footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
</style>
