<script setup lang="ts">
import { computed, ref } from 'vue'
import { Info, Pencil, Puzzle } from '@lucide/vue'
import type { ExtensionOption } from '../api'
import { useExtensionStore } from '../stores/extensions'
import ExtensionWindow from './ExtensionWindow.vue'
import ExtensionEditorDialog from './ExtensionEditorDialog.vue'
const props = defineProps<{ clientId?: string }>()
const ext = useExtensionStore()
const infoId = ref('')
const editId = ref('')
const error = ref('')
const options = computed(() => props.clientId ? ext.options.filter((option) => !option.clientId || option.clientId === props.clientId) : ext.options)
const info = computed(() => ext.options.find((option) => option.id === infoId.value))
const editing = computed(() => ext.options.find((option) => option.id === editId.value))
function descriptor(option: ExtensionOption) { return ext.types.find((type) => type.type === option.type) }
async function toggle(option: ExtensionOption, event: Event) {
  if (!props.clientId) return
  const current = option.clientId === props.clientId && option.enabled
  const input = event.target as HTMLInputElement
  input.checked = current
  error.value = ''
  try { await ext.enableOption(props.clientId, option.id, !current) }
  catch (err) { error.value = String(err) }
}
</script>
<template>
  <p v-if="ext.optionsError || error" class="list-error" role="alert">{{ error || ext.optionsError }}<button type="button" class="btn btn-ghost btn-sm" @click="ext.refreshOptions">重试</button></p>
  <p v-if="!ext.optionsLoaded" class="empty" role="status">正在加载扩展…</p>
  <p v-else-if="!options.length" class="empty">还没有可用的扩展。</p>
  <ul v-else class="options-list" aria-label="扩展列表">
    <li v-for="option in options" :key="option.id" class="option-row" :aria-busy="ext.pending.includes('extension:' + option.id)">
      <Puzzle :size="19" class="option-icon" aria-hidden="true" />
      <div class="option-identity"><strong>{{ option.name }}</strong><span>{{ descriptor(option)?.name ?? option.type }}</span><span v-if="!clientId && option.clientName">{{ option.clientName }} · {{ option.enabled ? '已启用' : '已停用' }}</span></div>
      <span v-for="field in descriptor(option)?.information.filter((value) => value.inList)" :key="field.key" class="inline-information">{{ option.values[field.key] || '—' }}</span>
      <input v-if="clientId" type="checkbox" class="switch" :checked="option.clientId === clientId && option.enabled" :disabled="ext.pending.includes('extension:' + option.id)" :aria-label="'启用扩展 ' + option.name" @change="toggle(option, $event)" />
      <div class="option-actions"><button type="button" class="btn btn-ghost row-action" :aria-label="'扩展信息 ' + option.name" title="扩展信息" @click="infoId = option.id"><Info :size="15" aria-hidden="true" /></button><button type="button" class="btn btn-ghost row-action" :disabled="ext.pending.includes('extension:' + option.id)" :aria-label="'编辑扩展 ' + option.name" title="编辑扩展" @click="editId = option.id"><Pencil :size="15" aria-hidden="true" /></button></div>
    </li>
  </ul>
  <ExtensionWindow :open="!!infoId" :title="(info?.name ?? '') + ' · 扩展信息'" @close="infoId = ''">
    <dl v-if="info" class="information-list"><div v-for="field in descriptor(info)?.information" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ info.values[field.key] || '—' }}</dd></div></dl>
    <p v-if="info?.lastError" class="list-error" role="alert">{{ info.lastError }}</p>
  </ExtensionWindow>
  <ExtensionEditorDialog :open="!!editId" :option="editing" :client-id="clientId" @close="editId = ''" />
</template>
<style scoped>
.options-list { padding: 0; margin: 0; list-style: none; border: 1px solid var(--outline); border-radius: 10px; background: var(--surface); overflow: hidden; }.option-row { display: flex; align-items: center; gap: 12px; padding: 16px; }.option-row + .option-row { border-top: 1px solid var(--outline); }.option-row:hover { background: var(--page); }.option-icon { flex-shrink: 0; color: var(--accent); }.option-identity { flex: 1; min-width: 0; display: grid; gap: 3px; }.option-identity strong { font-size: 14px; font-weight: 500; overflow-wrap: anywhere; }.option-identity span { color: var(--muted); font-size: 11px; }.inline-information { font-size: 12px; color: var(--muted); white-space: nowrap; }.option-actions { display: flex; gap: 4px; }.row-action { width: 28px; padding: 0; opacity: 0; transition: opacity 120ms ease; }.option-row:hover .row-action,.option-row:focus-within .row-action { opacity: 1; }
.empty { padding: 30px 12px; color: var(--muted); font-size: 13px; text-align: center; }.list-error { color: var(--danger); font-size: 13px; margin-bottom: 12px; }.information-list { margin: 0; display: grid; gap: 18px; }.information-list div { display: grid; gap: 5px; }.information-list dt { color: var(--muted); font-size: 12px; }.information-list dd { margin: 0; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; user-select: text; }
@media(hover:none) { .row-action { opacity: 1; } }@media(max-width:760px) { .option-row { flex-wrap: wrap; }.option-identity { min-width: 120px; }.option-actions { margin-left: auto; } }
</style>
