<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { ClearQQContext, DeleteQQContext, ListQQContexts, type QQContextInfo } from '../api'
import ExtensionWindow from './ExtensionWindow.vue'
import ConfirmationDialog from './ConfirmationDialog.vue'
const props = defineProps<{ open: boolean; clientId: string; extensionId: string; name: string }>()
const emit = defineEmits<{ close: [] }>()
const contexts = ref<QQContextInfo[]>([])
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const confirmation = ref<{ id: string; name: string; action: 'clear' | 'delete' }>()
let generation = 0
let pendingGeneration: number | undefined
let timer: ReturnType<typeof setInterval> | undefined
function stopPolling() { if (timer !== undefined) clearInterval(timer); timer = undefined }
onUnmounted(() => { generation++; stopPolling() })
async function load(background = false) {
  const current = generation
  if (pendingGeneration === current) return
  pendingGeneration = current
  if (!background) { loading.value = true; error.value = '' }
  try { const items = await ListQQContexts(props.clientId, props.extensionId); if (current === generation) { contexts.value = items; error.value = '' } }
  catch (err) { if (current === generation) error.value = String(err) }
  finally { if (current === generation) loading.value = false; if (pendingGeneration === current) pendingGeneration = undefined }
}
watch(() => [props.open, props.clientId, props.extensionId], () => {
  stopPolling()
  generation++; contexts.value = []; confirmation.value = undefined; error.value = ''; busy.value = false
  if (props.open) {
    void load()
    timer = setInterval(() => { if (!busy.value && !confirmation.value) void load(true) }, 2000)
  }
}, { immediate: true })
async function confirm() {
  if (!confirmation.value || busy.value) return
  const current = ++generation
  const target = confirmation.value
  const clientId = props.clientId, extensionId = props.extensionId
  busy.value = true; loading.value = false; error.value = ''
  try {
    const operation = target.action === 'delete' ? DeleteQQContext : ClearQQContext
    await operation(clientId, extensionId, target.id)
    if (current !== generation) return
    confirmation.value = undefined; await load()
  }
  catch (err) { if (current === generation) error.value = String(err) }
  finally { if (current === generation) busy.value = false }
}
function kind(item: QQContextInfo) { return item.kind === 'agent' ? 'agent 对话' : item.kind === 'group' ? '群共享对话' : item.kind === 'private' ? '私聊对话' : '个人对话（私聊 / 群聊共用）' }
function tokens(item: QQContextInfo) { return item.contextTokens == null ? '未提供' : item.contextTokens.toLocaleString('zh-CN') + ' tokens' }
</script>
<template>
  <ExtensionWindow :open="open" :title="'QQ 上下文 · ' + name" :busy="busy" wide @close="emit('close')">
    <div class="context-toolbar"><p>清除保留工作区文件并归档对话。删除会移除上下文、归档和整个工作区。两种操作都会取消旧任务，保留共享个人记忆。</p><div class="context-actions"><button type="button" class="btn btn-ghost btn-sm" :disabled="loading || busy || !contexts.length" @click="confirmation = { id: '', name: '全部 QQ 上下文', action: 'clear' }">清除全部</button><button type="button" class="btn btn-danger-outline btn-sm" :disabled="loading || busy || !contexts.length" @click="confirmation = { id: '', name: '全部 QQ 上下文', action: 'delete' }">删除全部</button></div></div>
    <p class="context-token-hint">当前长度取模型最近一次对话请求的输入 + 输出用量，包含系统和工具消息。达到 300,000 tokens 后会在 QQ 提醒一次，清除后重置。</p>
    <p v-if="loading" class="hint" role="status">正在加载上下文…</p>
    <div v-else class="contexts">
      <article v-for="item in contexts" :key="item.id" class="context"><div class="context-info"><h3>{{ item.workspace.name }}</h3><p>{{ kind(item) }} · 机器人 {{ item.botId }}<span v-if="item.userId"> · QQ {{ item.userId }}</span><span v-if="item.groupId"> · 群 {{ item.groupId }}</span></p><p class="context-tokens" :class="{ long: (item.contextTokens ?? 0) >= 300000 }">当前上下文长度：{{ tokens(item) }}<span v-if="(item.contextTokens ?? 0) >= 300000"> · 建议清除上下文</span></p><p class="id">{{ item.id }}</p><details><summary>内部工作区</summary><p>{{ item.workspace.path }}</p></details></div><div class="context-actions"><button type="button" class="btn btn-ghost btn-sm" :disabled="busy" @click="confirmation = { id: item.id, name: item.workspace.name, action: 'clear' }">清除上下文</button><button type="button" class="btn btn-danger-outline btn-sm" :disabled="busy" @click="confirmation = { id: item.id, name: item.workspace.name, action: 'delete' }">删除上下文</button></div></article>
      <p v-if="!contexts.length" class="hint">收到触发消息后会自动创建上下文。</p>
    </div>
    <p v-if="error && !confirmation" class="error" role="alert">{{ error }}<button type="button" class="btn btn-ghost btn-sm" @click="load()">重试</button></p>
  </ExtensionWindow>
  <ConfirmationDialog :open="!!confirmation" :title="confirmation?.action === 'delete' ? '删除 QQ 上下文' : '清除 QQ 上下文'" :message="confirmation?.action === 'delete' ? '确定删除「' + confirmation.name + '」？上下文、归档历史和整个内部工作区将永久删除，生成和排队消息会被取消。共享个人记忆保留。' : '确定清除「' + (confirmation?.name ?? '') + '」？旧对话将归档，生成和排队消息会被取消。工作区文件与长期记忆保留。'" :confirm-label="confirmation?.action === 'delete' ? '删除' : '清除'" danger :busy="busy" :error="error" @cancel="confirmation = undefined" @confirm="confirm" />
</template>
<style scoped>
.context-token-hint { margin: 0 0 15px; color: var(--muted); font-size: 12px; line-height: 1.7; }.context .context-tokens { font-variant-numeric: tabular-nums; }.context .context-tokens.long { color: var(--danger); }
.context-toolbar { display: flex; align-items: flex-start; gap: 14px; margin-bottom: 20px; }.context-toolbar p { flex: 1; margin: 0; color: var(--muted); font-size: 12px; line-height: 1.7; }.context-actions { display: flex; gap: 6px; flex-shrink: 0; flex-wrap: wrap; }.context-actions button { flex-shrink: 0; }.contexts { display: grid; gap: 10px; }.context { display: flex; align-items: flex-start; gap: 12px; padding: 15px; border: 1px solid var(--outline); border-radius: 9px; }.context-info { flex: 1; min-width: 0; }.context h3 { font-size: 13px; font-weight: 500; }.context p { margin: 7px 0 0; color: var(--muted); font-size: 12px; overflow-wrap: anywhere; line-height: 1.5; }.context .id { font-size: 11px; user-select: text; }.context details { margin-top: 8px; font-size: 12px; color: var(--muted); }.context summary { cursor: pointer; }.hint { color: var(--muted); font-size: 12px; }.error { color: var(--danger); font-size: 13px; }@media(max-width: 600px) { .context-toolbar,.context { flex-direction: column; } }
</style>
