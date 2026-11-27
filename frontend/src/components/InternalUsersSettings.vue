<script setup lang="ts">
import { computed, onMounted, ref, useId } from 'vue'
import { CreateInternalUser, ListInternalUsers, LoadConfigs, SetInternalUserModels, type Config, type InternalUser, type ModelGrant } from '../api'
import ExtensionWindow from './ExtensionWindow.vue'

const users = ref<InternalUser[]>([]), providers = ref<Config[]>([])
const editor = ref<'create' | 'edit' | null>(null), target = ref('')
const pass = ref(''), repeat = ref(''), selected = ref<ModelGrant[]>([])
const loading = ref(true), saving = ref(false), error = ref(''), notice = ref('')
const formId = useId()
const canSave = computed(() => !saving.value && !loading.value && (editor.value === 'edit' ||
  ([...pass.value].length >= 4 && pass.value === repeat.value)))
async function load() {
  loading.value = true; error.value = ''
  try {
    const [list, catalog] = await Promise.all([ListInternalUsers(), LoadConfigs()])
    users.value = list; providers.value = catalog.config ?? []
  } catch (e) { error.value = String(e) } finally { loading.value = false }
}
onMounted(load)
function open(user?: InternalUser) {
  editor.value = user ? 'edit' : 'create'; target.value = user?.id ?? ''
  selected.value = [...(user?.models ?? [])]; pass.value = repeat.value = error.value = notice.value = ''
}
function close() { editor.value = null; pass.value = repeat.value = error.value = ''; selected.value = [] }
function has(configId: string, modelId: string) { return selected.value.some(g => g.configId === configId && g.modelId === modelId) }
function toggle(configId: string, modelId: string) {
  selected.value = has(configId, modelId) ? selected.value.filter(g => g.configId !== configId || g.modelId !== modelId) : [...selected.value, { configId, modelId }]
}
async function save() {
  if (!canSave.value) return
  saving.value = true; error.value = ''
  try {
    if (editor.value === 'create') {
      const user = await CreateInternalUser(pass.value, selected.value); users.value.push(user)
      notice.value = '内部用户已创建。退出主用户后，对方可用自己的密码登录。'
    } else {
      const user = await SetInternalUserModels(target.value, selected.value)
      users.value = users.value.map(u => u.id === user.id ? user : u); notice.value = '模型权限已保存。'
    }
    close()
  } catch (e) { error.value = String(e) } finally { saving.value = false }
}
</script>
<template>
  <section class="members" aria-label="内部用户">
    <div class="members-head"><div><h2>内部用户</h2><p>为每个人分配可用模型。对方使用自己的密码登录，有独立的聊天记录，无需填写 API Key。</p></div>
      <button class="btn" :disabled="loading || !!error && !editor" @click="open()">创建内部用户</button></div>
    <p v-if="loading" role="status">正在读取用户…</p>
    <div v-else-if="error && !editor" role="alert">{{ error }} <button class="btn" @click="load">重试</button></div>
    <p v-else-if="!users.length" class="empty">还没有内部用户。</p>
    <ul v-else><li v-for="(user, i) in users" :key="user.id"><div><strong>内部用户 {{ i + 1 }}</strong><small>{{ user.id.slice(-8) }} · 已授权 {{ user.models.length }} 个模型</small></div>
      <button class="btn" :aria-label="`编辑内部用户 ${i + 1} 的模型权限`" @click="open(user)">模型权限</button></li></ul>
    <p v-if="notice" role="status">{{ notice }}</p>
    <ExtensionWindow :open="!!editor" :title="editor === 'create' ? '创建内部用户' : '编辑模型权限'" :busy="saving" @close="close">
      <form v-if="editor" :id="formId" @submit.prevent="save">
        <template v-if="editor === 'create'"><label class="field">登录密码（至少 4 个字符）<input v-model="pass" type="password" autocomplete="new-password" :disabled="saving" required /></label>
          <label class="field">再次输入密码<input v-model="repeat" type="password" autocomplete="new-password" :disabled="saving" required /></label>
          <p v-if="repeat && pass !== repeat">两次输入的密码不一致。</p></template>
        <div><strong>可使用的模型</strong><p>只勾选想共享的模型。全部留空时，用户仍可登录和查看自己的历史记录。</p></div>
        <fieldset v-for="cfg in providers" :key="cfg.ID" :disabled="saving"><legend>{{ cfg.Name || '未命名供应商' }}</legend>
          <label v-for="m in cfg.Model" :key="m.id" class="model"><input type="checkbox" :checked="has(cfg.ID, m.id)" @change="toggle(cfg.ID, m.id)" /><span>{{ m.name || m.id }}<small v-if="m.name && m.name !== m.id">{{ m.id }}</small></span></label>
        </fieldset>
        <p v-if="!providers.length">还没有模型。可以先创建用户，再到「模型设置」添加供应商后回来授权。</p>
        <p v-if="error" role="alert">{{ error }}</p>
      </form>
      <template #footer><button type="button" class="btn" :disabled="saving" @click="close">取消</button><button type="submit" class="btn btn-primary" :form="formId" :disabled="!canSave">{{ saving ? '正在保存…' : editor === 'create' ? '确认创建' : '保存权限' }}</button></template>
    </ExtensionWindow>
  </section>
</template>
<style scoped>
.members { padding: 18px 0; border-bottom: 1px solid var(--outline); } .members-head, li { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px 18px; } .members-head > div { flex: 1 1 240px; }
h2, p { margin: 0; } h2, strong, legend { font-size: 14px; } p, small { color: var(--muted); font-size: 13px; line-height: 1.5; } p { margin-top: 5px; } small { display: block; } .empty, ul { margin-top: 12px; } ul { padding: 0; margin-bottom: 0; } li { padding: 10px 0; border-top: 1px solid var(--outline); }
form, .field { display: grid; gap: 6px; } form { gap: 14px; } .field { font-size: 13px; } .field input { box-sizing: border-box; width: 100%; padding: 8px 10px; border: 1px solid var(--outline); border-radius: 6px; background: var(--surface); color: var(--ink); } fieldset { min-width: 0; margin: 0; padding: 6px 12px; border: 1px solid var(--outline); border-radius: 8px; } .model { display: flex; align-items: center; gap: 8px; padding: 5px 0; overflow-wrap: anywhere; } .model input { flex-shrink: 0; } [role=alert] { color: var(--danger); }
</style>
