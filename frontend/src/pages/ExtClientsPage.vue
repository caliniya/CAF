<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ChevronRight, Monitor, Plus, Puzzle } from '@lucide/vue'
import ExtensionClientDialog from '../components/ExtensionClientDialog.vue'
import { useExtensionStore } from '../stores/extensions'
import { useQQStore } from '../stores/qq'
const router = useRouter()
const ext = useExtensionStore()
const qq = useQQStore()
const open = ref(false)
const busy = ref(false)
const error = ref('')
let stop: (() => void) | undefined
onMounted(() => { void ext.refresh(); stop = qq.watchStatus() })
onUnmounted(() => stop?.())
function openCreate() { error.value = ''; open.value = true }
async function create(name: string, id: string) {
  busy.value = true; error.value = ''
  try { await ext.create(name, id); open.value = false }
  catch (err) { error.value = String(err) }
  finally { busy.value = false }
}
function status(adapterId?: string) { return qq.adapters.find((item) => item.id === adapterId)?.state }
</script>
<template>
  <section class="ext-page" aria-labelledby="ext-title">
    <header class="ext-head"><div><h1 id="ext-title">客户端</h1><p>管理客户端及其扩展</p></div><button type="button" class="btn btn-primary" @click="openCreate"><Plus :size="15" />新建客户端</button></header>
    <div class="ext-content">
      <div v-if="ext.error" class="page-error" role="alert">{{ ext.error }}<button type="button" class="btn btn-ghost" @click="ext.refresh">重试</button></div>
      <p v-if="ext.loading && !ext.loaded" class="hint" role="status">正在加载客户端…</p>
      <div v-else-if="!ext.clients.length && ext.loaded" class="empty"><Monitor :size="36" :stroke-width="1.3" /><h2>还没有客户端</h2><p>创建客户端，为它添加 QQ 等扩展。</p><button type="button" class="btn" @click="openCreate"><Plus :size="14" />新建客户端</button></div>
      <ul v-else class="ext-list" aria-label="扩展客户端">
        <li v-for="client in ext.clients" :key="client.id"><button type="button" class="ext-client" @click="router.push('/ext/clients/' + client.id)">
          <span class="avatar"><Monitor :size="20" /></span><span class="identity"><strong>{{ client.name }}</strong><span class="hint">{{ client.id }}</span>
            <span class="extensions"><span v-for="item in client.extensions" :key="item.adapterId" class="extension-tag" :class="{ connected: item.enabled && status(item.adapterId) === 'connected' }"><Puzzle :size="12" />QQ · {{ item.name }}<span>{{ !item.enabled ? '已停用' : status(item.adapterId) === 'connected' ? '已连接' : status(item.adapterId) === 'disconnected' ? '未连接' : '未启动' }}</span></span><span v-if="!client.extensions.length" class="hint">尚未添加扩展</span></span>
          </span><ChevronRight :size="17" class="chevron" />
        </button></li>
      </ul>
    </div>
    <ExtensionClientDialog :open="open" :busy="busy" :error="error" @cancel="open = false" @confirm="create" />
  </section>
</template>
<style scoped>
.ext-page { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; }
.ext-head { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 24px 28px 18px; border-bottom: 1px solid var(--outline); }
h1 { font-size: 20px; font-weight: 600; } .ext-head p { margin-top: 4px; color: var(--muted); font-size: 12px; }
.ext-content { flex: 1; min-height: 0; overflow-y: auto; padding: 24px 28px; }
.page-error { color: var(--danger); font-size: 13px; margin-bottom: 16px; }
.empty { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 72px 16px; color: var(--muted); text-align: center; }
.empty h2 { color: var(--ink); font-size: 16px; font-weight: 500; } .empty p { font-size: 13px; }
.hint { font-size: 12px; color: var(--muted); }
.ext-list { list-style: none; padding: 0; margin: 0; border: 1px solid var(--outline); border-radius: 12px; overflow: hidden; background: var(--surface); }
.ext-list li + li { border-top: 1px solid var(--outline); }
.ext-client { width: 100%; display: flex; align-items: center; gap: 16px; padding: 20px; border: 0; background: transparent; text-align: left; }
.ext-client:hover { background: var(--page); }
.avatar { display: grid; place-items: center; flex-shrink: 0; width: 40px; height: 40px; border-radius: 10px; background: var(--tonal); color: var(--accent); }
.identity { display: grid; flex: 1; min-width: 0; gap: 5px; } strong { font-size: 14px; font-weight: 500; overflow-wrap: anywhere; }
.extensions { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 3px; }.extension-tag { display: inline-flex; align-items: center; flex-wrap: wrap; gap: 5px; padding: 3px 7px; border-radius: 5px; background: var(--page); font-size: 11px; color: var(--muted); }
.extension-tag.connected { background: var(--success-soft); color: var(--success); }.chevron { flex-shrink: 0; color: var(--muted); }
@media(max-width: 900px) { .ext-head, .ext-content { padding: 20px; } }
</style>
