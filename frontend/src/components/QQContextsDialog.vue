<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { ClearQQContext, ListQQContexts, type QQContextInfo } from '../api'
import ExtensionWindow from './ExtensionWindow.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
const props = defineProps<{ open: boolean; clientId: string; extensionId: string; name: string }>()
const emit = defineEmits<{ close: [] }>()
const contexts = ref<QQContextInfo[]>([])
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const confirmation = ref<{ id: string; name: string }>()
let generation = 0
onUnmounted(() => { generation++ })
async function load() {
  const current = generation
  loading.value = true; error.value = ''
  try { const items = await ListQQContexts(props.clientId, props.extensionId); if (current === generation) contexts.value = items }
  catch (err) { if (current === generation) error.value = String(err) }
  finally { if (current === generation) loading.value = false }
}
watch(() => [props.open, props.clientId, props.extensionId], () => {
  generation++; contexts.value = []; confirmation.value = undefined; error.value = ''
  if (props.open) void load()
}, { immediate: true })
async function clear() {
  if (!confirmation.value || busy.value) return
  busy.value = true; error.value = ''
  try { await ClearQQContext(props.clientId, props.extensionId, confirmation.value.id); confirmation.value = undefined; await load() }
  catch (err) { error.value = String(err) }
  finally { busy.value = false }
}
function kind(item: QQContextInfo) { return item.kind === 'agent' ? 'agent 对话' : item.kind === 'group' ? '群共享对话' : item.kind === 'private' ? '私聊对话' : '个人对话（私聊 / 群聊共用）' }
</script>
<template>
  <ExtensionWindow :open="open" :title="'QQ 上下文 · ' + name" :busy="busy" wide @close="emit('close')">
    <div class="context-toolbar"><p>清除会取消当前生成和排队消息，工作区文件与长期记忆保留。</p><button type="button" class="btn btn-danger-outline btn-sm" :disabled="loading || busy || !contexts.length" @click="confirmation = { id: '', name: '全部 QQ 上下文' }">清除全部</button></div>
    <p v-if="loading" class="hint" role="status">正在加载上下文…</p>
    <div v-else class="contexts">
      <article v-for="item in contexts" :key="item.id" class="context"><div><h3>{{ item.workspace.name }}</h3><p>{{ kind(item) }} · 机器人 {{ item.botId }}<span v-if="item.userId"> · QQ {{ item.userId }}</span><span v-if="item.groupId"> · 群 {{ item.groupId }}</span></p><p class="id">{{ item.id }}</p><details><summary>内部工作区</summary><p>{{ item.workspace.path }}</p></details></div><button type="button" class="btn btn-ghost btn-sm" :disabled="busy" @click="confirmation = { id: item.id, name: item.workspace.name }">清除上下文</button></article>
      <p v-if="!contexts.length" class="hint">收到触发消息后会自动创建上下文。</p>
    </div>
    <p v-if="error && !confirmation" class="error" role="alert">{{ error }}<button type="button" class="btn btn-ghost btn-sm" @click="load">重试</button></p>
  </ExtensionWindow>
  <ConfirmationDialog :open="!!confirmation" title="清除 QQ 上下文" :message="'确定清除「' + (confirmation?.name ?? '') + '」？旧对话将归档，生成和排队消息会被取消。工作区文件与长期记忆保留。'" confirm-label="清除" danger :busy="busy" :error="error" @cancel="confirmation = undefined" @confirm="clear" />
</template>
<style scoped>
.context-toolbar { display: flex; align-items: flex-start; gap: 14px; margin-bottom: 20px; }.context-toolbar p { flex: 1; margin: 0; color: var(--muted); font-size: 12px; line-height: 1.7; }.context-toolbar button { flex-shrink: 0; }.contexts { display: grid; gap: 10px; }.context { display: flex; align-items: flex-start; gap: 12px; padding: 15px; border: 1px solid var(--outline); border-radius: 9px; }.context>div { flex: 1; min-width: 0; }.context h3 { font-size: 13px; font-weight: 500; }.context p { margin: 7px 0 0; color: var(--muted); font-size: 12px; overflow-wrap: anywhere; line-height: 1.5; }.context .id { font-size: 11px; user-select: text; }.context details { margin-top: 8px; font-size: 12px; color: var(--muted); }.context summary { cursor: pointer; }.hint { color: var(--muted); font-size: 12px; }.error { color: var(--danger); font-size: 13px; }@media(max-width: 600px) { .context-toolbar,.context { flex-direction: column; } }
</style>
