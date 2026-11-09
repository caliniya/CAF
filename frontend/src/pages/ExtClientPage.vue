<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from '@lucide/vue'
import { GetExtensionClient } from '../api'
import { useExtensionStore } from '../stores/extensions'
import ConfirmationDialog from '../components/ConfirmationDialog.vue'
const route = useRoute(); const router = useRouter(); const ext = useExtensionStore()
const id = computed(() => String(route.params.clientId))
const client = computed(() => ext.clients.find((value) => value.id === id.value))
const name = ref(''); const loading = ref(true); const error = ref(''); const saved = ref(false); const deleteOpen = ref(false)
const busy = computed(() => ext.pending.includes(id.value))
let generation = 0
onUnmounted(() => { generation++ })
async function load() { const current = ++generation; loading.value = true; error.value = ''; try { const result = await GetExtensionClient(id.value); if (current !== generation) return; ext.replace(result); name.value = result.name } catch (err) { if (current === generation) error.value = String(err) } finally { if (current === generation) loading.value = false } }
watch(id, () => { saved.value = false; deleteOpen.value = false; void load() }, { immediate:true })
watch(name, () => { saved.value = false })
async function save() { error.value = ''; try { await ext.rename(id.value, name.value); saved.value = true } catch (err) { error.value = String(err) } }
async function remove() { error.value = ''; try { await ext.remove(id.value); await router.push('/ext') } catch (err) { error.value = String(err) } }
</script>
<template>
  <section class="client-edit-page"><header><button type="button" class="btn btn-ghost" aria-label="返回客户端列表" :disabled="busy" @click="router.push('/ext')"><ArrowLeft :size="17" /></button><h1>编辑客户端</h1></header>
    <div class="edit-content"><p v-if="loading" class="hint">正在加载客户端…</p><p v-if="error && !deleteOpen" class="error" role="alert">{{ error }}</p><button v-if="!loading && !client" type="button" class="btn" @click="load">重试</button>
      <form v-if="client && !loading" class="client-form" @submit.prevent="save"><label>ID<input :value="client.id" type="text" readonly /></label><label>名称<input v-model="name" type="text" required :disabled="busy" /></label><p v-if="saved" class="saved" role="status">名称已保存</p><div class="save-actions"><button type="submit" class="btn btn-primary" :disabled="busy || !name.trim()">保存修改</button></div>
      <div class="delete-section"><div><h2>删除客户端</h2><p class="hint">关闭所属扩展，归档内部数据并释放 ID。</p></div><button type="button" class="btn btn-danger-outline" :disabled="busy" @click="error = ''; deleteOpen = true">删除客户端</button></div></form>
    </div><ConfirmationDialog :open="deleteOpen" title="删除客户端" :message="'确定删除「' + (client?.name ?? '') + '」？所属扩展会关闭，内部数据将归档。'" confirm-label="删除" danger :busy="busy" :error="error" @cancel="deleteOpen = false" @confirm="remove" />
  </section>
</template>
<style scoped>.client-edit-page { display:flex;flex-direction:column;flex:1;min-height:0;min-width:0; }header { display:flex;align-items:center;gap:12px;padding:24px 28px 18px;border-bottom:1px solid var(--outline); }header .btn { width:30px;padding:0; }h1 { font-size:20px; }.edit-content { flex:1;min-height:0;padding:24px 28px;overflow-y:auto; }.client-form { max-width:580px;display:grid;gap:20px; }label { display:grid;gap:7px;font-size:13px; }input { height:38px;padding:0 11px;border:1px solid var(--outline);border-radius:8px;background:var(--surface); }input[readonly] { background:var(--page);color:var(--muted); }.hint { color:var(--muted);font-size:12px; }.error { color:var(--danger);font-size:13px; }.saved { color:var(--success);font-size:12px; }.save-actions { display:flex;justify-content:flex-end; }.delete-section { display:flex;align-items:center;justify-content:space-between;gap:14px;margin-top:16px;padding-top:22px;border-top:1px solid var(--outline); }h2 { font-size:14px;margin-bottom:5px; }@media(max-width:900px) { header,.edit-content { padding:20px; } }</style>
