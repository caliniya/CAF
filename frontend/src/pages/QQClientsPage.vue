<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Monitor, Pencil, Play, Plus, Square } from '@lucide/vue'
import type { QQAdapterInput } from '../api'
import QQAdapterDialog from '../components/QQAdapterDialog.vue'
import { QQ_STATE_LABELS, useQQStore } from '../stores/qq'

const router = useRouter()
const qq = useQQStore()
const createOpen = ref(false)
const createBusy = ref(false)
const createError = ref('')
let stopWatching: (() => void) | undefined
onMounted(() => { stopWatching = qq.watchStatus() })
onUnmounted(() => stopWatching?.())

function openCreate() { createError.value = ''; createOpen.value = true }
async function create(input: QQAdapterInput) {
  createBusy.value = true
  createError.value = ''
  try {
    await qq.create(input)
    createOpen.value = false
  } catch (err) { createError.value = String(err) }
  finally { createBusy.value = false }
}
</script>

<template>
  <section class="clients-page" aria-labelledby="clients-title">
    <header class="clients-head">
      <h1 id="clients-title">客户端</h1>
      <button type="button" class="btn btn-primary" @click="openCreate">
        <Plus :size="15" aria-hidden="true" />新建适配器
      </button>
    </header>
    <div class="clients-content">
      <div v-if="qq.error" class="clients-error" role="alert">
        <span>{{ qq.error }}</span>
        <button type="button" class="btn btn-ghost btn-sm" :disabled="qq.loading" @click="qq.refresh()">重试</button>
      </div>
      <p v-if="qq.loading && !qq.loaded" class="loading-message" role="status">正在加载客户端…</p>
      <div v-else-if="!qq.adapters.length && qq.loaded" class="clients-empty">
        <Monitor :size="34" :stroke-width="1.3" aria-hidden="true" />
        <h2>还没有客户端</h2>
        <p>新建适配器后，即可连接 QQ 客户端。</p>
        <button type="button" class="btn" @click="openCreate"><Plus :size="14" aria-hidden="true" />新建适配器</button>
      </div>
      <ul v-else-if="qq.adapters.length" class="client-list" aria-label="QQ 客户端">
        <li v-for="adapter in qq.adapters" :key="adapter.id" class="client-row" :aria-busy="qq.pending.includes(adapter.id)">
          <div class="client-avatar"><Monitor :size="20" :stroke-width="1.5" aria-hidden="true" /></div>
          <div class="client-info">
            <div class="client-identity">
              <span class="client-name" :title="adapter.name">{{ adapter.name }}</span>
              <span v-if="adapter.qq" class="client-qq">QQ {{ adapter.qq }}</span>
            </div>
            <span class="client-address">{{ adapter.address }}</span>
            <span v-if="adapter.lastError" class="client-last-error">{{ adapter.lastError }}</span>
          </div>
          <span class="client-state" :class="adapter.state" role="status">
            <span class="state-dot" aria-hidden="true"></span>{{ QQ_STATE_LABELS[adapter.state] }}
          </span>
          <div class="client-actions">
            <button type="button" class="btn btn-ghost client-edit" :aria-label="'编辑客户端 ' + adapter.name"
              title="编辑客户端" :disabled="qq.pending.includes(adapter.id)"
              @click="router.push('/qq/clients/' + adapter.id)">
              <Pencil :size="15" aria-hidden="true" />
            </button>
            <button type="button" class="btn client-toggle" :class="{ 'btn-primary': adapter.state === 'stopped' }"
              :disabled="qq.pending.includes(adapter.id)" :aria-label="(adapter.state === 'stopped' ? '启动 ' : '关闭 ') + adapter.name"
              @click="qq.toggle(adapter)">
              <Play v-if="adapter.state === 'stopped'" :size="14" aria-hidden="true" />
              <Square v-else :size="14" aria-hidden="true" />
              {{ qq.pending.includes(adapter.id) ? '处理中…' : adapter.state === 'stopped' ? '启动' : '关闭' }}
            </button>
          </div>
        </li>
      </ul>
    </div>
    <QQAdapterDialog :open="createOpen" :busy="createBusy" :error="createError"
      @cancel="createOpen = false" @confirm="create" />
  </section>
</template>

<style scoped>
.clients-page { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; }
.clients-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 24px 28px 18px; border-bottom: 1px solid var(--outline); }
.clients-head h1 { font-size: 20px; font-weight: 600; }
.clients-content { flex: 1; min-height: 0; overflow-y: auto; padding: 24px 28px; }
.clients-error { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px; color: var(--danger); font-size: 13px; }
.loading-message { color: var(--muted); font-size: 13px; }
.clients-empty { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 78px 16px; color: var(--muted); text-align: center; }
.clients-empty h2 { color: var(--ink); font-size: 16px; font-weight: 500; }
.clients-empty p { font-size: 13px; }
.clients-empty .btn { margin-top: 6px; }
.client-list { margin: 0; padding: 0; list-style: none; border: 1px solid var(--outline); border-radius: 12px; background: var(--surface); overflow: hidden; }
.client-row { display: flex; align-items: center; gap: 16px; padding: 18px 20px; transition: background-color 120ms ease; }
.client-row + .client-row { border-top: 1px solid var(--outline); }
.client-row:hover { background: color-mix(in srgb, var(--page) 55%, var(--surface)); }
.client-avatar { display: grid; place-items: center; flex-shrink: 0; width: 38px; height: 38px; border-radius: 10px; background: var(--tonal); color: var(--accent); }
.client-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.client-identity { display: flex; align-items: baseline; flex-wrap: wrap; gap: 8px; min-width: 0; }
.client-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; font-weight: 500; }
.client-qq { color: var(--muted); font-size: 12px; }
.client-address { color: var(--muted); font-family: var(--font-mono); font-size: 11px; overflow-wrap: anywhere; }
.client-last-error { color: var(--danger); font-size: 12px; }
.client-state { display: inline-flex; align-items: center; gap: 6px; flex-shrink: 0; color: var(--muted); font-size: 12px; }
.state-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.client-state.connected { color: var(--success); }
.client-state.disconnected { color: var(--warning); }
.client-actions { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
.client-toggle { min-width: 70px; }
.client-edit { width: 30px; padding: 0; opacity: 0; transition: opacity 120ms ease; }
.client-row:hover .client-edit, .client-row:focus-within .client-edit { opacity: 1; }
@media (hover: none) { .client-edit { opacity: 1; } }
@media (max-width: 900px) {
  .clients-head { padding: 20px; }
  .clients-content { padding: 20px; }
  .client-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; padding: 16px; }
  .client-avatar { display: none; }
  .client-info { grid-column: 1 / -1; }
  .client-actions { justify-self: end; }
}
</style>
