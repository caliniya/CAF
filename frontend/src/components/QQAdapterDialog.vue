<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import { X } from '@lucide/vue'
import { GetQQAdapterAddress, type QQAdapterInput } from '../api'
import QQAdapterFields from './QQAdapterFields.vue'

const props = defineProps<{ open: boolean; busy: boolean; error: string }>()
const emit = defineEmits<{ confirm: [input: QQAdapterInput]; cancel: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const input = ref<QQAdapterInput>({ name: '', token: '', selfId: '' })
const address = ref('')
const addressError = ref('')
const titleId = useId()
let generation = 0

function cancel() { if (!props.busy) emit('cancel') }
function confirm() {
  if (!props.busy && address.value && input.value.name.trim() && input.value.token.trim()) {
    emit('confirm', { ...input.value })
  }
}

async function loadAddress() {
  const currentGeneration = ++generation
  addressError.value = ''
  try {
    const result = await GetQQAdapterAddress()
    if (generation === currentGeneration && props.open) address.value = result
  } catch (err) {
    if (generation === currentGeneration && props.open) addressError.value = String(err)
  }
}

watch(() => props.open, async (open) => {
  generation++
  if (open) {
    input.value = { name: '', token: '', selfId: '' }
    address.value = ''
    void loadAddress()
  } else {
    input.value.token = ''
  }
  await nextTick()
  if (props.open !== open) return
  if (open && dialog.value && !dialog.value.open) dialog.value.showModal()
  else if (!open && dialog.value?.open) dialog.value.close()
}, { immediate: true })
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="adapter-dialog" :aria-labelledby="titleId" :aria-busy="busy"
      @click.self="cancel" @cancel.prevent="cancel" @keydown.stop>
      <form @submit.prevent="confirm">
        <header class="adapter-dialog-head">
          <h2 :id="titleId">新建 QQ 适配器</h2>
          <button type="button" class="btn btn-ghost close-button" aria-label="关闭创建窗口"
            :disabled="busy" @click="cancel"><X :size="16" aria-hidden="true" /></button>
        </header>
        <div class="adapter-dialog-content">
          <QQAdapterFields v-model="input" :address="address" :busy="busy" autofocus />
          <p v-if="error || addressError" class="adapter-error" role="alert">{{ error || addressError }}</p>
          <button v-if="addressError" type="button" class="btn btn-ghost btn-sm" @click="loadAddress">重新获取地址</button>
        </div>
        <footer class="adapter-dialog-actions">
          <button type="button" class="btn btn-ghost" :disabled="busy" @click="cancel">取消</button>
          <button type="submit" class="btn btn-primary"
            :disabled="busy || !address || !input.name.trim() || !input.token.trim()">
            {{ busy ? '创建中…' : '创建适配器' }}
          </button>
        </footer>
      </form>
    </dialog>
  </Teleport>
</template>

<style scoped>
.adapter-dialog {
  width: min(540px, calc(100vw - 40px)); max-height: calc(100vh - 40px); padding: 0;
  border: 1px solid var(--outline); border-radius: 14px; background: var(--surface);
  color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
}
.adapter-dialog::backdrop { background: rgb(22 29 50 / 42%); }
.adapter-dialog-head { display: flex; align-items: center; justify-content: space-between; padding: 16px 20px; border-bottom: 1px solid var(--outline); }
.adapter-dialog-head h2 { font-size: 16px; font-weight: 600; }
.close-button { width: 30px; padding: 0; }
.adapter-dialog-content { padding: 22px 20px; }
.adapter-error { margin-top: 14px; color: var(--danger); font-size: 13px; }
.adapter-dialog-actions { display: flex; justify-content: flex-end; gap: 8px; padding: 16px 20px; border-top: 1px solid var(--outline); }
</style>
