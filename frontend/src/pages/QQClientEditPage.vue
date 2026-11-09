<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Trash2 } from '@lucide/vue'
import { GetQQAdapter, type QQAdapter, type QQAdapterInput } from '../api'
import QQAdapterFields from '../components/QQAdapterFields.vue'
import ConfirmationDialog from '../components/ConfirmationDialog.vue'
import { QQ_STATE_LABELS, useQQStore } from '../stores/qq'

const route = useRoute()
const router = useRouter()
const qq = useQQStore()
const backPath = computed(() => '/ext/clients/' + String(route.params.clientId))
const details = ref<QQAdapter | null>(null)
const input = ref<QQAdapterInput>({ name: '', token: '', selfId: '' })
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const saved = ref(false)
const deleteOpen = ref(false)
const deleteError = ref('')
const adapter = computed(() => qq.adapters.find((item) => item.id === details.value?.id) ?? details.value)
const connectionLocked = computed(() => adapter.value?.state !== 'stopped')
let stopWatching: (() => void) | undefined
let generation = 0
onMounted(() => { stopWatching = qq.watchStatus() })
onUnmounted(() => { generation++; stopWatching?.() })

async function load() {
  const currentGeneration = ++generation
  loading.value = true
  error.value = ''
  details.value = null
  input.value = { name: '', token: '', selfId: '' }
  saved.value = false
  deleteOpen.value = false
  try {
    const result = await GetQQAdapter(String(route.params.id))
    if (generation !== currentGeneration) return
    details.value = result
    input.value = { name: result.name, token: result.token ?? '', selfId: result.selfId }
    delete result.token
  } catch (err) { if (generation === currentGeneration) error.value = String(err) }
  finally { if (generation === currentGeneration) loading.value = false }
}
watch(() => route.params.id, () => void load(), { immediate: true })
watch(input, () => { saved.value = false }, { deep: true })

async function save() {
  if (!details.value || busy.value) return
  busy.value = true
  error.value = ''
  saved.value = false
  try {
    details.value = await qq.update(details.value.id, input.value)
    saved.value = true
  } catch (err) { error.value = String(err) }
  finally { busy.value = false }
}

async function remove() {
  if (!details.value || busy.value) return
  busy.value = true
  deleteError.value = ''
  try {
    await qq.remove(details.value.id)
    deleteOpen.value = false
    await router.push(backPath.value)
  } catch (err) { deleteError.value = String(err) }
  finally { busy.value = false }
}
</script>

<template>
  <section class="edit-page" aria-labelledby="edit-title">
    <header class="edit-head">
      <button type="button" class="btn btn-ghost back-button" aria-label="返回客户端"
        :disabled="busy" @click="router.push(backPath)"><ArrowLeft :size="18" aria-hidden="true" /></button>
      <h1 id="edit-title">QQ 适配器配置</h1>
    </header>
    <div class="edit-content">
      <p v-if="loading" class="edit-hint" role="status">正在加载适配器…</p>
      <div v-else-if="!details">
        <p class="edit-error" role="alert">{{ error }}</p>
        <button type="button" class="btn" @click="load">重试</button>
      </div>
      <form v-else class="edit-form" @submit.prevent="save">
        <div class="edit-summary">
          <span class="edit-client-name">{{ adapter?.name }}</span>
          <span v-if="adapter?.qq" class="edit-hint">QQ {{ adapter.qq }}</span>
          <span v-if="adapter" class="edit-state" :class="adapter.state" role="status">{{ QQ_STATE_LABELS[adapter.state] }}</span>
        </div>
        <QQAdapterFields v-model="input" :address="details.address" :busy="busy" :connection-locked="connectionLocked" />
        <p v-if="error" class="edit-error" role="alert">{{ error }}</p>
        <p v-if="saved" class="saved-message" role="status">修改已保存</p>
        <div class="edit-actions">
          <button type="button" class="btn btn-ghost" :disabled="busy" @click="router.push(backPath)">返回客户端</button>
          <button type="submit" class="btn btn-primary" :disabled="busy || !input.name.trim() || !input.token.trim()">
            {{ busy ? '处理中…' : '保存修改' }}
          </button>
        </div>
        <div class="delete-section">
          <div><h2>删除适配器</h2><p class="edit-hint">请先在客户端中移除扩展绑定，再删除适配器。</p></div>
          <button type="button" class="btn btn-danger-outline" :disabled="busy"
            @click="deleteError = ''; deleteOpen = true"><Trash2 :size="14" aria-hidden="true" />删除</button>
        </div>
      </form>
    </div>
    <ConfirmationDialog :open="deleteOpen" title="删除适配器" confirm-label="删除" danger
      :message="'确定删除适配器「' + (details?.name ?? '') + '」？运行中的连接会关闭，适配器配置将被删除。'"
      :busy="busy" :error="deleteError" @cancel="deleteOpen = false" @confirm="remove" />
  </section>
</template>

<style scoped>
.edit-page { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; }
.edit-head { display: flex; align-items: center; gap: 12px; padding: 24px 28px 18px; border-bottom: 1px solid var(--outline); }
.edit-head h1 { font-size: 20px; font-weight: 600; }
.back-button { width: 30px; padding: 0; margin-left: -7px; }
.edit-content { flex: 1; min-height: 0; padding: 24px 28px; overflow-y: auto; }
.edit-form { width: 100%; max-width: 620px; }
.edit-summary { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-bottom: 24px; padding-bottom: 18px; border-bottom: 1px solid var(--outline); }
.edit-client-name { font-size: 15px; font-weight: 500; }
.edit-hint { color: var(--muted); font-size: 12px; }
.edit-state { margin-left: auto; color: var(--muted); font-size: 12px; }
.edit-state.connected { color: var(--success); }
.edit-state.disconnected { color: var(--warning); }
.edit-error { margin: 16px 0; color: var(--danger); font-size: 13px; }
.saved-message { margin-top: 16px; color: var(--success); font-size: 13px; }
.edit-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 24px; }
.delete-section { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 32px; padding-top: 22px; border-top: 1px solid var(--outline); }
.delete-section h2 { margin-bottom: 5px; font-size: 14px; font-weight: 500; }
@media (max-width: 900px) { .edit-head, .edit-content { padding: 20px; } }
</style>
