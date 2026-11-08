<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import { Folder } from '@lucide/vue'
import { SelectDirectory } from '../api'

const props = defineProps<{
  open: boolean
  initialDirectory?: string
  busy: boolean
  error: string
}>()
const emit = defineEmits<{
  confirm: [name: string, path: string]
  cancel: []
}>()
const dialog = ref<HTMLDialogElement | null>(null)
const name = ref('')
const path = ref('')
const browsing = ref(false)
const browseError = ref('')
const titleId = useId()

function cancel() {
  if (!props.busy && !browsing.value) emit('cancel')
}

async function chooseDirectory() {
  if (browsing.value || props.busy) return
  browsing.value = true
  browseError.value = ''
  try {
    const selected = await SelectDirectory(path.value.trim() || props.initialDirectory || '')
    if (selected) {
      path.value = selected
      if (!name.value.trim()) {
        name.value = selected.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || selected
      }
    }
  } catch (err) {
    browseError.value = String(err)
  } finally {
    browsing.value = false
  }
}

function confirm() {
  if (!props.busy && !browsing.value && path.value.trim()) {
    emit('confirm', name.value.trim(), path.value.trim())
  }
}

watch(
  () => props.open,
  async (open) => {
    if (open) {
      name.value = ''
      path.value = ''
      browseError.value = ''
    }
    await nextTick()
    if (props.open !== open) return
    if (open && dialog.value && !dialog.value.open) dialog.value.showModal()
    else if (!open && dialog.value?.open) dialog.value.close()
  },
  { immediate: true },
)
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="workspace-dialog" :aria-labelledby="titleId"
      @click.self="cancel" @cancel.prevent="cancel" @keydown.stop>
      <form @submit.prevent="confirm">
        <header class="workspace-dialog-head">
          <h2 :id="titleId">新建工作区</h2>
        </header>
        <div class="workspace-dialog-content">
          <label class="workspace-field">
            <span>名称</span>
            <input v-model="name" type="text" placeholder="留空使用文件夹名称" autofocus
              :disabled="busy || browsing" />
          </label>
          <label class="workspace-field">
            <span>路径</span>
            <span class="workspace-path-field">
              <input v-model="path" type="text" placeholder="选择工作区文件夹" required
                :disabled="busy || browsing" />
              <button type="button" class="btn btn-ghost" :disabled="busy || browsing"
                @click="chooseDirectory">
                <Folder :size="15" :stroke-width="2" />
                {{ browsing ? '选择中…' : '选择文件夹' }}
              </button>
            </span>
          </label>
          <p v-if="error || browseError" class="workspace-dialog-error" role="alert">{{ error || browseError }}</p>
        </div>
        <footer class="workspace-dialog-actions">
          <button type="button" class="btn btn-ghost" :disabled="busy || browsing" @click="cancel">取消</button>
          <button type="submit" class="btn btn-primary"
            :disabled="busy || browsing || !path.trim()">{{ busy ? '创建中…' : '创建并使用' }}</button>
        </footer>
      </form>
    </dialog>
  </Teleport>
</template>

<style scoped>
.workspace-dialog {
  width: min(520px, calc(100vw - 40px));
  max-height: calc(100vh - 40px);
  padding: 0;
  border: 1px solid var(--outline);
  border-radius: 14px;
  background: var(--surface);
  color: var(--ink);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
}

.workspace-dialog::backdrop {
  background: rgb(22 29 50 / 42%);
}

.workspace-dialog-head {
  padding: 18px 20px;
  border-bottom: 1px solid var(--outline);
}

.workspace-dialog-head h2 {
  font-size: 16px;
  font-weight: 600;
}

.workspace-dialog-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 20px;
}

.workspace-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 13px;
}

.workspace-field > span:first-child {
  color: var(--muted);
}

.workspace-field input {
  width: 100%;
  min-width: 0;
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--outline);
  border-radius: 8px;
  background: var(--surface);
  color: var(--ink);
  font-size: 13px;
}

.workspace-path-field {
  display: flex;
  gap: 8px;
}

.workspace-path-field input {
  flex: 1;
}

.workspace-path-field button {
  flex-shrink: 0;
  white-space: nowrap;
}

.workspace-dialog-error {
  color: var(--danger);
  font-size: 13px;
}

.workspace-dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 20px;
  border-top: 1px solid var(--outline);
}
</style>
