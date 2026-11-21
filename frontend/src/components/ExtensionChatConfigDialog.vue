<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { Bot, Check, ChevronLeft, ChevronRight, ShieldCheck, UserRound, Wrench } from '@lucide/vue'
import { GetClientExtensionChatConfig, UpdateClientExtensionChatConfig, ListPersonas, ListToolSets, type ExtensionChatConfig, type Persona, type ToolSetDescriptor } from '../api'
import { useConfigsStore } from '../stores/configs'
import ExtensionWindow from './ExtensionWindow.vue'

const props = defineProps<{ open: boolean; clientId: string; extensionId: string; name: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const providers = useConfigsStore()
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const config = ref<ExtensionChatConfig>()
const personas = ref<Persona[]>([])
const sets = ref<ToolSetDescriptor[]>([])
const tab = ref<'main' | 'model' | 'persona' | 'tools'>('main')
const selectedProvider = computed(() => providers.configs.find(provider => provider.ID === config.value?.configId))
const selectedPersona = computed(() => personas.value.find(persona => persona.id === config.value?.personaId))
const valid = computed(() => !!config.value && !!selectedProvider.value?.Model.some(model => model.id === config.value?.model) && !!selectedPersona.value)
let generation = 0
onUnmounted(() => { generation++ })
watch(() => [props.open, props.clientId, props.extensionId] as const, async ([open]) => {
  const current = ++generation
  config.value = undefined; error.value = ''; tab.value = 'main'; loading.value = false
  if (!open) return
  loading.value = true
  try {
    if (!providers.loaded) await providers.load()
    if (!providers.loaded) throw new Error(providers.error || '模型配置尚未加载')
    const [value, roles, tools] = await Promise.all([GetClientExtensionChatConfig(props.clientId, props.extensionId), ListPersonas(), ListToolSets()])
    if (current !== generation) return
    config.value = { ...value, toolConfig: { ...value.toolConfig, permission: 2, strictMode: value.toolConfig?.strictMode ?? false, sets: structuredClone(value.toolConfig?.sets ?? {}) } }
    personas.value = roles; sets.value = tools
  } catch (err) { if (current === generation) error.value = String(err) }
  finally { if (current === generation) loading.value = false }
}, { immediate: true })

function model(configId: string, modelId: string) { if (config.value) { config.value.configId = configId; config.value.model = modelId } }
function kitValue(setId: string, kitId: string, defaultValue: boolean) {
  return config.value?.toolConfig?.sets?.[setId]?.kits?.[kitId] ?? providers.toolNormalConfig?.sets?.[setId]?.kits?.[kitId] ?? defaultValue
}
function setKit(setId: string, kitId: string, event: Event) {
  const value = config.value
  if (!value) return
  value.toolConfig ??= { permission: 2 }
  value.toolConfig.sets ??= {}
  const set = value.toolConfig.sets[setId] ??= { kits: {} }
  set.kits ??= {}
  set.kits[kitId] = (event.target as HTMLInputElement).checked
}
function strict() {
  if (!config.value) return
  config.value.toolConfig ??= { permission: 2 }
  config.value.toolConfig.strictMode = !config.value.toolConfig.strictMode
}
function resetTools() { if (config.value?.toolConfig) config.value.toolConfig.sets = {} }
async function save() {
  if (!config.value || !valid.value || busy.value) return
  const current = generation
  busy.value = true; error.value = ''
  try {
    await UpdateClientExtensionChatConfig(props.clientId, props.extensionId, JSON.parse(JSON.stringify(config.value)) as ExtensionChatConfig)
    if (current === generation) { emit('saved'); emit('close') }
  } catch (err) { if (current === generation) error.value = String(err) }
  finally { busy.value = false }
}
</script>

<template>
  <ExtensionWindow :open="open" :title="'对话配置 · ' + name" :busy="busy" @close="emit('close')">
    <p v-if="loading" class="hint" role="status">正在加载对话配置…</p>
    <template v-else-if="config">
      <button v-if="tab !== 'main'" type="button" class="btn btn-ghost btn-sm back" @click="tab = 'main'"><ChevronLeft :size="15" />返回配置</button>
      <div v-if="tab === 'main'" class="config-menu">
        <button type="button" class="config-row" @click="tab = 'model'"><Bot :size="18" /><span>模型配置<small>{{ selectedProvider?.Name || selectedProvider?.ServerAddress || '未选择供应商' }} · {{ config.model || '未选择模型' }}</small></span><ChevronRight :size="16" /></button>
        <button type="button" class="config-row" @click="tab = 'persona'"><UserRound :size="18" /><span>人设配置<small>{{ selectedPersona?.name ?? '未选择人设' }}</small></span><ChevronRight :size="16" /></button>
        <button type="button" class="config-row" @click="tab = 'tools'"><Wrench :size="18" /><span>工具配置<small>使用自动编辑权限，在对应的内部工作区执行</small></span><ChevronRight :size="16" /></button>
        <button type="button" class="config-row" role="switch" :aria-checked="config.toolConfig?.strictMode ?? false" @click="strict"><ShieldCheck :size="18" /><span>严格模式<small>权限不足时要求模型主动申请执行</small></span><Check v-if="config.toolConfig?.strictMode" :size="16" /></button>
      </div>
      <div v-else-if="tab === 'model'" class="choices">
        <section v-for="provider in providers.configs" :key="provider.ID" class="choice-group">
          <h3>{{ provider.Name || provider.ServerAddress }}</h3>
          <button v-for="item in provider.Model" :key="item.id" type="button" class="choice" :class="{ selected: config.configId === provider.ID && config.model === item.id }" @click="model(provider.ID, item.id)"><span>{{ item.name || item.id }}<small v-if="item.name && item.name !== item.id">{{ item.id }}</small></span><Check v-if="config.configId === provider.ID && config.model === item.id" :size="16" /></button>
        </section>
        <p v-if="!providers.configs.length" class="hint">请先在模型页面添加供应商和模型。</p>
        <label class="reasoning">思考强度<select v-model="config.reasoningEffort"><option value="low">低</option><option value="medium">中</option><option value="high">高</option></select></label>
      </div>
      <div v-else-if="tab === 'persona'" class="choices">
        <button v-for="item in personas" :key="item.id" type="button" class="choice" :class="{ selected: config.personaId === item.id }" @click="config.personaId = item.id"><span>{{ item.name }}<small>{{ item.description }}</small></span><Check v-if="config.personaId === item.id" :size="16" /></button>
      </div>
      <div v-else class="choices">
        <section v-for="set in sets" :key="set.id" class="choice-group"><h3>{{ set.name }}</h3>
          <label v-for="kit in set.kits" :key="kit.id" class="choice"><span>{{ kit.name }}<small>{{ kit.description }}</small></span><input type="checkbox" :checked="kitValue(set.id, kit.id, set.defaultEnabled && kit.defaultEnabled)" @change="setKit(set.id, kit.id, $event)" /></label>
        </section>
        <button type="button" class="btn btn-ghost btn-sm" @click="resetTools">恢复默认工具配置</button>
      </div>
      <p class="hint">保存后会取消正在进行的旧请求；已有上下文保留，新消息使用新配置。</p>
    </template>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <template #footer><button type="button" class="btn btn-ghost" :disabled="busy" @click="emit('close')">取消</button><button type="button" class="btn btn-primary" :disabled="busy || loading || !valid" @click="save">{{ busy ? '保存中…' : '保存配置' }}</button></template>
  </ExtensionWindow>
</template>

<style scoped>
.config-menu,.choices { display: grid; gap: 8px; }.config-row { display: flex; align-items: center; gap: 12px; width: 100%; padding: 14px 12px; border: 1px solid var(--outline); border-radius: 9px; background: var(--surface); text-align: left; font-size: 13px; }.config-row:hover,.choice:hover { background: var(--tonal); }.config-row>span,.choice>span { flex: 1; min-width: 0; }.config-row small,.choice small { display: block; margin-top: 5px; color: var(--muted); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }.hint { margin: 17px 0 0; color: var(--muted); font-size: 12px; line-height: 1.7; }.error { margin-top: 15px; color: var(--danger); font-size: 13px; }.back { margin-bottom: 12px; }.choice-group h3 { margin: 14px 0 8px; color: var(--muted); font-size: 12px; }.choice { display: flex; align-items: center; gap: 12px; width: 100%; padding: 11px 12px; border: 0; border-radius: 8px; background: transparent; text-align: left; font-size: 13px; cursor: pointer; }.choice.selected { color: var(--accent); background: var(--tonal); }.reasoning { display: flex; align-items: center; justify-content: space-between; margin-top: 15px; font-size: 13px; }.reasoning select { min-width: 120px; padding: 6px 10px; border: 1px solid var(--outline); border-radius: 7px; background: var(--surface); }.choice input { flex-shrink: 0; accent-color: var(--accent); }
</style>
