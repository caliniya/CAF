<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { X } from '@lucide/vue'
const props = defineProps<{ open: boolean; busy: boolean; error: string }>()
const emit = defineEmits<{ confirm: [name: string, id: string]; cancel: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const name = ref('')
const id = ref('')
const titleId = useId()
const valid = computed(() => !!name.value.trim() && /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$/.test(id.value))
function cancel() { if (!props.busy) emit('cancel') }
watch(() => props.open, async (open) => {
  if (open) { name.value = ''; id.value = '' }
  await nextTick()
  if (props.open !== open) return
  if (open && dialog.value && !dialog.value.open) dialog.value.showModal()
  else if (!open && dialog.value?.open) dialog.value.close()
}, { immediate: true })
</script>
<template>
  <Teleport to="body">
    <dialog ref="dialog" class="client-dialog" :aria-labelledby="titleId" :aria-busy="busy"
      @click.self="cancel" @cancel.prevent="cancel" @keydown.stop>
      <form @submit.prevent="valid && !busy && emit('confirm', name.trim(), id)">
        <header><h2 :id="titleId">新建客户端</h2><button type="button" class="btn btn-ghost" aria-label="关闭窗口" :disabled="busy" @click="cancel"><X :size="16" /></button></header>
        <div class="client-dialog-content">
          <label><span>名称</span><input v-model="name" type="text" placeholder="客户端名称" required autofocus :disabled="busy" /></label>
          <label><span>ID</span><input v-model="id" type="text" placeholder="例如 my-client" pattern="[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}" required :disabled="busy" /></label>
          <p class="hint">ID 创建后固定，支持字母、数字、下划线和短横线。客户端数据由应用保存。</p>
          <p v-if="error" class="error" role="alert">{{ error }}</p>
        </div>
        <footer><button type="button" class="btn btn-ghost" :disabled="busy" @click="cancel">取消</button><button type="submit" class="btn btn-primary" :disabled="busy || !valid">{{ busy ? '创建中…' : '创建客户端' }}</button></footer>
      </form>
    </dialog>
  </Teleport>
</template>
<style scoped>
.client-dialog { width: min(480px, calc(100vw - 40px)); max-height: calc(100vh - 40px); padding: 0; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); color: var(--ink); box-shadow: 0 24px 70px rgb(22 29 50 / 26%); }
.client-dialog::backdrop { background: rgb(22 29 50 / 42%); }
header, footer { display: flex; align-items: center; gap: 8px; padding: 16px 20px; }
header { justify-content: space-between; border-bottom: 1px solid var(--outline); }
header h2 { font-size: 16px; } header .btn { width: 30px; padding: 0; }
footer { justify-content: flex-end; border-top: 1px solid var(--outline); }
.client-dialog-content { display: grid; gap: 18px; padding: 22px 20px; }
label { display: grid; gap: 7px; font-size: 13px; }
input { width: 100%; height: 38px; padding: 0 11px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); }
.hint { color: var(--muted); font-size: 12px; } .error { color: var(--danger); font-size: 13px; }
</style>
