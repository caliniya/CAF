<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'

const props = defineProps<{
  open: boolean
  title: string
  message: string
  confirmLabel: string
  danger?: boolean
  busy?: boolean
  error?: string
}>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const titleId = useId()
const messageId = `${titleId}-message`

function cancel() {
  if (!props.busy) emit('cancel')
}

watch(
  () => props.open,
  async (open) => {
    await nextTick()
    if (props.open !== open) return
    if (open && dialog.value && !dialog.value.open) {
      dialog.value.showModal()
    } else if (!open && dialog.value?.open) {
      dialog.value.close()
    }
  },
  { immediate: true },
)
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="confirmation-dialog" role="alertdialog"
      :aria-labelledby="titleId" :aria-describedby="messageId" :aria-busy="busy"
      @click.self="cancel" @cancel.prevent="cancel" @keydown.stop>
      <h2 :id="titleId">{{ title }}</h2>
      <p :id="messageId" class="confirmation-message">{{ message }}</p>
      <p v-if="error" class="confirmation-error" role="alert">{{ error }}</p>
      <footer class="confirmation-actions">
        <button type="button" class="btn btn-ghost" :disabled="busy" autofocus @click="cancel">取消</button>
        <button type="button" class="btn" :class="danger ? 'btn-danger' : 'btn-primary'"
          :disabled="busy" @click="emit('confirm')">{{ busy ? '处理中…' : confirmLabel }}</button>
      </footer>
    </dialog>
  </Teleport>
</template>

<style scoped>
.confirmation-dialog {
  width: min(420px, calc(100vw - 40px));
  max-height: calc(100vh - 40px);
  padding: 20px;
  border: 1px solid var(--outline);
  border-radius: 14px;
  background: var(--surface);
  color: var(--ink);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
}

.confirmation-dialog::backdrop {
  background: rgb(22 29 50 / 42%);
}

.confirmation-dialog h2 {
  font-size: 16px;
  font-weight: 600;
}

.confirmation-message {
  margin-top: 12px;
  color: var(--muted);
  font-size: 13px;
  white-space: pre-wrap;
}

.confirmation-error {
  margin-top: 12px;
  color: var(--danger);
  font-size: 13px;
}

.confirmation-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 20px;
}
</style>
