<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Pencil, Plus, Puzzle, Trash2 } from '@lucide/vue'
import { GetExtensionClient, type ClientExtension, type QQAdapterInput } from '../api'
import QQAdapterDialog from '../components/QQAdapterDialog.vue'
import ConfirmationDialog from '../components/ConfirmationDialog.vue'
import { useExtensionStore } from '../stores/extensions'
import { QQ_STATE_LABELS, useQQStore } from '../stores/qq'
const route = useRoute(); const router = useRouter()
const ext = useExtensionStore(); const qq = useQQStore()
const id = computed(() => String(route.params.clientId))
const client = computed(() => ext.clients.find((item) => item.id === id.value))
const name = ref(''); const error = ref(''); const saved = ref(false); const loading = ref(true)
const addOpen = ref(false); const extensionName = ref(''); const adapterId = ref(''); const addError = ref('')
const createOpen = ref(false); const createBusy = ref(false); const createError = ref(''); const deleteOpen = ref(false)
const busy = computed(() => ext.pending.includes(id.value) || createBusy.value)
const available = computed(() => qq.adapters.filter((adapter) => !ext.clients.some((owner) => owner.extensions.some((item) => item.adapterId === adapter.id))))
const validExtensionName = computed(() => /^[\p{L}\p{N}_-]{1,32}$/u.test(extensionName.value.trim()))
let stop: (() => void) | undefined; let generation = 0
onMounted(() => { stop = qq.watchStatus(); void ext.refresh() })
onUnmounted(() => { generation++; stop?.() })
async function load() {
  const current = ++generation; loading.value = true; error.value = ''; saved.value = false; addOpen.value = false
  try { const result = await GetExtensionClient(id.value); if (current !== generation) return; ext.replace(result); name.value = result.name }
  catch (err) { if (current === generation) error.value = String(err) }
  finally { if (current === generation) loading.value = false }
}
watch(id, () => void load(), { immediate: true })
function adapter(item: ClientExtension) { return qq.adapters.find((value) => value.id === item.adapterId) }
async function rename() { error.value = ''; saved.value = false; try { await ext.rename(id.value, name.value); saved.value = true } catch (err) { error.value = String(err) } }
function openAdd() { addOpen.value = true; extensionName.value = ''; adapterId.value = ''; addError.value = '' }
async function add() { addError.value = ''; try { await ext.setQQ(id.value, adapterId.value, extensionName.value, true); addOpen.value = false } catch (err) { addError.value = String(err) } }
async function toggle(item: ClientExtension) { error.value = ''; try { await ext.setQQ(id.value, item.adapterId!, item.name, !item.enabled); await qq.refresh(true) } catch (err) { error.value = String(err) } }
async function start(item: ClientExtension) { error.value = ''; try { await ext.setQQ(id.value, item.adapterId!, item.name, true); await qq.refresh(true) } catch (err) { error.value = String(err) } }
async function removeQQ(item: ClientExtension) { error.value = ''; try { await ext.removeQQ(id.value, item.adapterId!); await qq.refresh(true) } catch (err) { error.value = String(err) } }
async function createAdapter(input: QQAdapterInput) {
  createBusy.value = true; createError.value = ''
  try { const result = await qq.create(input); adapterId.value = result.id; createOpen.value = false }
  catch (err) { createError.value = String(err) } finally { createBusy.value = false }
}
async function remove() { error.value = ''; try { await ext.remove(id.value); await router.push('/ext') } catch (err) { error.value = String(err) } }
</script>
<template>
  <section class="detail-page" aria-labelledby="detail-title">
    <header class="detail-head"><button type="button" class="btn btn-ghost" aria-label="返回客户端" :disabled="busy" @click="router.push('/ext')"><ArrowLeft :size="17" /></button><div><h1 id="detail-title">{{ client?.name || '客户端' }}</h1><p v-if="client">ID：{{ client.id }}</p></div></header>
    <div class="detail-content">
      <p v-if="loading" class="hint" role="status">正在加载客户端…</p>
      <p v-if="error && !deleteOpen" class="error" role="alert">{{ error }}</p>
      <button v-if="!loading && !client" type="button" class="btn" @click="load">重新加载</button>
      <template v-if="client && !loading">
        <form class="name-form" @submit.prevent="rename"><label>名称<input v-model="name" type="text" required :disabled="busy" /></label><button type="submit" class="btn" :disabled="busy || !name.trim()">保存名称</button><span v-if="saved" class="saved" role="status">已保存</span></form>
        <div class="section-head"><h2>扩展</h2><button type="button" class="btn btn-primary" :disabled="busy" @click="openAdd"><Plus :size="14" />添加 QQ 扩展</button></div>
        <p v-if="!client.extensions.length" class="hint no-extensions">为客户端添加 QQ 扩展，使用你填写的名称作为指令前缀。</p>
        <ul class="extension-list">
          <li v-for="item in client.extensions" :key="item.adapterId" class="extension-row">
            <Puzzle :size="20" class="extension-icon" /><div class="extension-info"><strong>QQ · {{ item.name }}</strong><span class="hint">/{{ item.name }} help · /{{ item.name }} status</span><span v-if="adapter(item)?.qq" class="hint">QQ {{ adapter(item)?.qq }}</span><span v-if="adapter(item)" class="address">{{ adapter(item)?.address }}</span><span v-if="adapter(item)?.lastError" class="error">{{ adapter(item)?.lastError }}</span></div>
            <div class="extension-controls"><span class="hint">{{ !item.enabled ? '已停用' : QQ_STATE_LABELS[adapter(item)?.state ?? 'stopped'] }}</span>
              <input class="switch" type="checkbox" :checked="item.enabled" :aria-label="'启用 QQ 扩展 ' + item.name" :disabled="busy" @change="toggle(item)" />
              <button v-if="item.enabled && adapter(item)?.state === 'stopped'" type="button" class="btn btn-sm" :disabled="busy" @click="start(item)">启动</button>
              <button type="button" class="btn btn-ghost icon-button" :aria-label="'配置 QQ 扩展 ' + item.name" :disabled="busy" @click="router.push('/ext/clients/' + id + '/qq/' + item.adapterId)"><Pencil :size="14" /></button>
              <button type="button" class="btn btn-ghost icon-button" :aria-label="'移除 QQ 扩展 ' + item.name" :disabled="busy" @click="removeQQ(item)"><Trash2 :size="14" /></button>
            </div>
          </li>
        </ul>
        <form v-if="addOpen" class="add-extension" @submit.prevent="add">
          <h3>添加 QQ 扩展</h3><label>扩展名称<input v-model="extensionName" type="text" required placeholder="例如 小助手" :disabled="busy" /></label>
          <p class="hint">名称必填，使用 /{{ extensionName || '名称' }} 识别指令。同一客户端可以添加多个 QQ 扩展。</p>
          <label>QQ 适配器<select v-model="adapterId" required :disabled="busy"><option value="" disabled>选择适配器</option><option v-for="value in available" :key="value.id" :value="value.id">{{ value.name }}{{ value.qq ? ' · QQ ' + value.qq : '' }}</option></select></label>
          <button type="button" class="btn btn-ghost" :disabled="busy" @click="createError = ''; createOpen = true"><Plus :size="14" />新建 QQ 适配器</button>
          <p v-if="addError" class="error" role="alert">{{ addError }}</p><div class="add-actions"><button type="button" class="btn btn-ghost" :disabled="busy" @click="addOpen = false">取消</button><button type="submit" class="btn btn-primary" :disabled="busy || !validExtensionName || !adapterId">添加并启用</button></div>
        </form>
        <div class="delete-client"><div><h2>删除客户端</h2><p class="hint">关闭所属扩展并归档客户端数据。</p></div><button type="button" class="btn btn-danger-outline" :disabled="busy" @click="error = ''; deleteOpen = true">删除</button></div>
      </template>
    </div>
    <QQAdapterDialog :open="createOpen" :busy="createBusy" :error="createError" @cancel="createOpen = false" @confirm="createAdapter" />
    <ConfirmationDialog :open="deleteOpen" title="删除客户端" :message="'确定删除「' + (client?.name ?? '') + '」？QQ 扩展会关闭，内部数据将归档。'" confirm-label="删除" danger :busy="busy" :error="error" @cancel="deleteOpen = false" @confirm="remove" />
  </section>
</template>
<style scoped>
.detail-page { display: flex; flex-direction: column; flex: 1; min-height: 0; min-width: 0; }
.detail-head { display: flex; align-items: center; gap: 10px; padding: 24px 28px 18px; border-bottom: 1px solid var(--outline); }.detail-head .btn { width: 30px; padding: 0; }.detail-head h1 { font-size: 20px; }.detail-head p { font-size: 12px; color: var(--muted); margin-top: 3px; }
.detail-content { flex: 1; min-height: 0; overflow-y: auto; padding: 24px 28px; }.hint { color: var(--muted); font-size: 12px; }.error { color: var(--danger); font-size: 13px; }.saved { color: var(--success); font-size: 12px; }
label { display: grid; gap: 7px; font-size: 13px; }input[type=text], select { width: 100%; min-width: 0; height: 38px; padding: 0 11px; border: 1px solid var(--outline); border-radius: 8px; background: var(--surface); }
.name-form { display: flex; align-items: flex-end; flex-wrap: wrap; gap: 10px; max-width: 600px; margin-bottom: 30px; }.name-form label { flex: 1; min-width: 160px; }.name-form .btn { height: 38px; }
.section-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 20px 0 16px; }h2 { font-size: 15px; font-weight: 500; }h3 { font-size: 14px; font-weight: 500; }
.no-extensions { padding: 24px; border: 1px dashed var(--outline); border-radius: 10px; }.extension-list { list-style: none; margin: 0; padding: 0; }.extension-row { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; padding: 18px; margin-bottom: 10px; border: 1px solid var(--outline); border-radius: 10px; background: var(--surface); }.extension-icon { color: var(--accent); flex-shrink: 0; }.extension-info { display: grid; gap: 4px; flex: 1; min-width: 150px; }strong { font-size: 14px; font-weight: 500; }.address { font-family: var(--font-mono); font-size: 11px; color: var(--muted); overflow-wrap: anywhere; }.extension-controls { display: flex; align-items: center; flex-wrap: wrap; gap: 9px; }.icon-button { width: 28px; padding: 0; }
.add-extension { display: grid; gap: 14px; max-width: 580px; margin-top: 18px; padding: 20px; border: 1px solid var(--outline); border-radius: 12px; background: var(--surface); }.add-extension > .btn { justify-self: start; }.add-actions { display: flex; justify-content: flex-end; gap: 8px; }
.delete-client { display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-top: 36px; padding-top: 22px; border-top: 1px solid var(--outline); }.delete-client p { margin-top: 5px; }
@media(max-width:900px) { .detail-head, .detail-content { padding: 20px; }.extension-info { flex-basis: calc(100% - 40px); }.extension-controls { margin-left: 32px; } }
</style>
