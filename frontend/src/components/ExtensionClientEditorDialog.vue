<script setup lang="ts">
import { onUnmounted, ref, useId, watch } from 'vue'
import type { ExtensionClient } from '../api'
import { useExtensionStore } from '../stores/extensions'
import ExtensionWindow from './ExtensionWindow.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'

const props = defineProps<{ open: boolean; client: ExtensionClient }>()
const emit = defineEmits<{ close: []; deleted: [clientId: string] }>()
const ext = useExtensionStore()
const name = ref('')
const busy = ref(false)
const error = ref('')
const deleteOpen = ref(false)
const formId = useId()
let generation = 0

watch(() => [props.open, props.client.id] as const, ([open]) => {
  generation++
  error.value = ''; deleteOpen.value = false
  if (open) name.value = props.client.name
}, { immediate: true })
onUnmounted(() => { generation++ })

async function save() {
  if (busy.value || !name.value.trim()) return
  const current = generation
  busy.value = true; error.value = ''
  try {
    await ext.rename(props.client.id, name.value.trim())
    if (current === generation) emit('close')
  } catch (err) {
    if (current === generation) error.value = String(err)
  } finally {
    if (current === generation) busy.value = false
  }
}

async function remove() {
  if (busy.value) return
  const current = generation, clientId = props.client.id
  busy.value = true; error.value = ''
  try {
    await ext.remove(clientId)
    if (current !== generation) return
    deleteOpen.value = false
    emit('deleted', clientId); emit('close')
  } catch (err) {
    if (current === generation) error.value = String(err)
  } finally {
    if (current === generation) busy.value = false
  }
}
</script>

<template>
  <ExtensionWindow :open="open" :title="'编辑客户端 · ' + client.name" :busy="busy" @close="emit('close')">
    <form :id="formId" class="client-editor-form" @submit.prevent="save">
      <label><span>ID</span><input :value="client.id" type="text" readonly /></label>
      <label><span>名称</span><input v-model="name" type="text" required autofocus :disabled="busy" /></label>
    </form>
    <p v-if="error && !deleteOpen" class="client-editor-error" role="alert">{{ error }}</p>
    <div class="client-editor-delete"><div><h3>删除客户端</h3><p>关闭所属扩展，归档内部数据并释放 ID。</p></div><button type="button" class="btn btn-danger-outline" :disabled="busy" @click="error = ''; deleteOpen = true">删除客户端</button></div>
    <template #footer><button type="button" class="btn btn-ghost" :disabled="busy" @click="emit('close')">取消</button><button type="submit" class="btn btn-primary" :form="formId" :disabled="busy || !name.trim()">{{ busy ? '处理中…' : '保存修改' }}</button></template>
  </ExtensionWindow>
  <ConfirmationDialog :open="deleteOpen" title="删除客户端" :message="'确定删除「' + client.name + '」？所属扩展会关闭，内部数据将归档。'" confirm-label="删除" danger :busy="busy" :error="error" @cancel="deleteOpen = false" @confirm="remove" />
</template>

<style scoped>
.client-editor-form { display: grid; gap: 20px; }
.client-editor-form label { display: grid; gap: 7px; font-size: 13px; }
.client-editor-form input { width: 100%; height: 38px; padding: 0 11px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); }
.client-editor-form input[readonly] { background: var(--page); color: var(--muted); }
.client-editor-error { margin-top: 16px; color: var(--danger); font-size: 13px; }
.client-editor-delete { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 28px; padding-top: 20px; border-top: 1px solid var(--outline); }
.client-editor-delete h3 { margin-bottom: 5px; font-size: 14px; font-weight: 500; }
.client-editor-delete p { color: var(--muted); font-size: 12px; }
.client-editor-delete .btn { flex-shrink: 0; }
@media(max-width: 480px) { .client-editor-delete { align-items: flex-start; flex-direction: column; } }
</style>
