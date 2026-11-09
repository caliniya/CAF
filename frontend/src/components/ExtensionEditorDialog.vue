<script setup lang="ts">
import { computed, onUnmounted, ref, useId, watch } from 'vue'
import { GetExtensionParameters, type ExtensionOption } from '../api'
import { useExtensionStore } from '../stores/extensions'
import ExtensionWindow from './ExtensionWindow.vue'
import ExtensionParameterFields from './ExtensionParameterFields.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
const props = defineProps<{ open: boolean; option?: ExtensionOption; clientId?: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const ext = useExtensionStore()
const selectedType = ref('')
const values = ref<Record<string, string>>({})
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const deleteOpen = ref(false)
const descriptor = computed(() => ext.types.find((type) => type.type === (props.option?.type ?? selectedType.value)))
const formId = useId()
let generation = 0
onUnmounted(() => { generation++ })
const valid = computed(() => !!descriptor.value && descriptor.value.parameters.every((field) => {
  const value = values.value[field.key] ?? ''
  if (!value.trim()) return !field.required
  if (field.kind === 'integer' && (!Number.isInteger(Number(value)) || Number(value) < (field.minimum ?? -Infinity) || Number(value) > (field.maximum ?? Infinity))) return false
  if (field.pattern) { try { if (!new RegExp(field.pattern, 'u').test(value)) return false } catch { /* native backend validates unsupported patterns */ } }
  return true
}))
watch(selectedType, () => {
  if (props.option) return
  values.value = Object.fromEntries(descriptor.value?.parameters.map((field) => [field.key, field.default ?? '']) ?? [])
  // 使用扩展声明的端口范围，跳过当前实例占用的端口。
  const portField = descriptor.value?.parameters.find((field) => field.key === 'port' && field.kind === 'integer')
  if (portField) {
    const used = new Set(ext.options.map((option) => Number(option.values.address?.match(/:(\d+)\//)?.[1])))
    let port = Number(values.value.port)
    while (used.has(port) && port < (portField.maximum ?? 65535)) port++
    values.value.port = String(port)
  }
})
watch(() => props.open, async (open) => {
  const current = ++generation
  error.value = ''; deleteOpen.value = false; selectedType.value = ''; values.value = {}
  loading.value = false
  if (!open || !props.option) return
  loading.value = true
  try { const result = await GetExtensionParameters(props.option.id); if (current === generation) values.value = result }
  catch (err) { if (current === generation) error.value = String(err) }
  finally { if (current === generation) loading.value = false }
}, { immediate: true })
async function save() {
  if (!valid.value || busy.value) return
  busy.value = true; error.value = ''
  try {
    if (props.option) await ext.updateOption(props.option.id, values.value)
    else await ext.createOption(selectedType.value, values.value)
    emit('saved'); emit('close')
  } catch (err) { error.value = String(err) }
  finally { busy.value = false }
}
async function remove() {
  if (!props.option) return
  busy.value = true; error.value = ''
  try { await ext.deleteOption(props.option.id); deleteOpen.value = false; emit('close') }
  catch (err) { error.value = String(err) }
  finally { busy.value = false }
}
async function detach() {
  if (!props.clientId || !props.option) return
  busy.value = true; error.value = ''
  try { await ext.detachOption(props.clientId, props.option.id); emit('close') }
  catch (err) { error.value = String(err) }
  finally { busy.value = false }
}
</script>
<template>
  <ExtensionWindow :open="open" :title="option ? '编辑扩展 · ' + option.name : '添加新的扩展'" :busy="busy" @close="emit('close')">
    <p v-if="loading" class="muted" role="status">正在加载参数…</p>
    <form v-else :id="formId" @submit.prevent="save">
      <label v-if="!option" class="type-select">扩展类型<select v-model="selectedType" required :disabled="busy"><option value="" disabled>选择要创建的扩展</option><option v-for="type in ext.types" :key="type.type" :value="type.type">{{ type.name }}</option></select></label>
      <p v-if="descriptor?.description" class="description">{{ descriptor.description }}</p>
      <ExtensionParameterFields v-if="descriptor" :fields="descriptor.parameters" :values="values" :busy="busy" :running="option?.running" @change="(key, value) => values[key] = value" />
      <p v-if="option?.clientName" class="owner">所属客户端：{{ option.clientName }}</p>
    </form>
    <p v-if="error && !deleteOpen" class="error" role="alert">{{ error }}</p>
    <template #footer>
      <button v-if="option && clientId && option.clientId === clientId" type="button" class="btn btn-danger-outline" :disabled="busy" @click="detach">移出客户端</button>
      <button v-else-if="option && !option.clientId" type="button" class="btn btn-danger-outline" :disabled="busy" @click="error = ''; deleteOpen = true">删除扩展</button>
      <span class="spacer"></span><button type="button" class="btn btn-ghost" :disabled="busy" @click="emit('close')">取消</button>
      <button type="submit" class="btn btn-primary" :form="formId" :disabled="busy || loading || !valid">{{ busy ? '处理中…' : option ? '保存修改' : '创建扩展' }}</button>
    </template>
  </ExtensionWindow>
  <ConfirmationDialog :open="deleteOpen" title="删除扩展" :message="'确定删除扩展「' + (option?.name ?? '') + '」？'" confirm-label="删除" danger :busy="busy" :error="error" @cancel="deleteOpen = false" @confirm="remove" />
</template>
<style scoped>
.type-select { display: grid; gap: 8px; font-size: 13px; }select { width: 100%; height: 38px; padding: 0 10px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); }.description { margin: 14px 0 18px; color: var(--muted); font-size: 12px; }.muted,.owner { color: var(--muted); font-size: 12px; }.owner { margin-top: 18px; }.error { margin-top: 14px; color: var(--danger); font-size: 13px; }.spacer { flex: 1; }
</style>
