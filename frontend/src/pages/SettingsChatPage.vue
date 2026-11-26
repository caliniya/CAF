<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { GetChatDefaults, GetChatMemorySettings, ListToolSets, ListWorkspaces, type ChatDefaults, type ChatToolConfig, type MemoryLibrary, type ToolSetDescriptor, type Workspace } from '../api'
import { useConfigsStore } from '../stores/configs'
import { memoryScopeLabel } from '../memory'

const configs = useConfigsStore()
const draft = ref<ChatDefaults | null>(null)
const tools = ref<ToolSetDescriptor[]>([])
const libraries = ref<MemoryLibrary[]>([])
const workspaces = ref<Workspace[]>([])
const busy = ref(false)
const error = ref('')
const saved = ref('')
const baseline = ref('')
const dirty = computed(() => draft.value !== null && JSON.stringify(draft.value) !== baseline.value)
const valid = computed(() => {
  const p = draft.value?.compaction
  return p && Number.isInteger(p.windowPercent) && p.windowPercent >= 1 && p.windowPercent <= 100 && Number.isInteger(p.tokenThreshold) && p.tokenThreshold >= 1 && p.tokenThreshold <= 2147483647
})
function apply(value: ChatDefaults) {
  draft.value = { ...value, tools: value.tools ?? {}, memoryLibraryIds: [...(value.memoryLibraryIds ?? [])] }
  baseline.value = JSON.stringify(draft.value)
}
async function load() {
  busy.value = true; error.value = ''
  try {
    const [defaults, sets, memory, spaces] = await Promise.all([GetChatDefaults(), ListToolSets(), GetChatMemorySettings(''), ListWorkspaces()])
    configs.applyChatDefaults(defaults)
    tools.value = sets; libraries.value = memory.libraries; workspaces.value = spaces; apply(defaults)
  } catch (err) { error.value = String(err) }
  finally { busy.value = false }
}
onMounted(load)
function changeKit(setId: string, kitId: string, fallback: boolean, checked: boolean) {
  const config = draft.value?.tools
  if (!config) return
  const sets: NonNullable<ChatToolConfig['sets']> = config.sets ??= {}
  const entry = sets[setId] ??= { kits: {} }
  const kits = entry.kits ??= {}
  if (checked === fallback) delete kits[kitId]; else kits[kitId] = checked
  if (!Object.keys(kits).length) delete sets[setId]
  if (!Object.keys(sets).length) delete config.sets
}
async function save() {
  if (!draft.value || !valid.value || busy.value) return
  busy.value = true; error.value = ''; saved.value = ''
  try { apply(await configs.saveChatDefaults(draft.value)); saved.value = '对话默认设置已保存。' }
  catch (err) { error.value = String(err) }
  finally { busy.value = false }
}
</script>

<template>
  <section class="page"><div class="content">
    <header><h1>对话设置</h1><p>设置工具、记忆和上下文压缩的全局默认值。</p></header>
    <p v-if="error" class="error" role="alert">{{ error }} <button v-if="!draft" type="button" class="btn btn-ghost btn-sm" @click="load">重试</button></p>
    <p v-if="!draft && busy" role="status">正在读取设置…</p>
    <form v-if="draft && draft.tools" @submit.prevent="save">
      <fieldset :disabled="busy"><legend>默认工具配置</legend>
        <p>未单独配置的对话沿用这些权限和工具模块。</p>
        <label class="row"><span>默认权限</span><select :value="draft.tools.permission ?? 2" @change="draft.tools.permission = Number(($event.target as HTMLSelectElement).value) as 1|2|3|4">
          <option :value="1">只读</option><option :value="2">自动编辑</option><option :value="3" v-if="draft.tools.permission === 3">自动审批</option><option :value="4">完全访问</option>
        </select></label>
        <label class="row"><span>启用严格模式<small>工具权限不足时，由模型主动申请执行。</small></span><input type="checkbox" class="switch" v-model="draft.tools.strictMode" /></label>
        <details v-for="set in tools" :key="set.id"><summary>{{ set.name }} <small>{{ set.kits.length }} 个模块</small></summary>
          <label v-for="kit in set.kits" :key="kit.id" class="row nested"><span>{{ kit.name }}<small>{{ kit.description }}</small></span>
            <input type="checkbox" :checked="draft.tools.sets?.[set.id]?.kits?.[kit.id] ?? kit.defaultEnabled" @change="changeKit(set.id, kit.id, kit.defaultEnabled, ($event.target as HTMLInputElement).checked)" />
          </label>
        </details>
      </fieldset>
      <fieldset :disabled="busy"><legend>默认记忆设置</legend>
        <p>新建对话时启用勾选的记忆。已有对话保留自己的选择。</p>
        <label v-for="library in libraries" :key="library.id" class="row"><span>{{ library.name === 'Main' ? '全局主记忆' : library.name }}<small>{{ memoryScopeLabel(library, workspaces) }}</small></span>
          <input type="checkbox" :value="library.id" v-model="draft.memoryLibraryIds" /></label>
        <p v-if="!libraries.length">暂无可选记忆。</p>
      </fieldset>
      <fieldset :disabled="busy"><legend>上下文压缩</legend>
        <label class="row"><span>启用上下文压缩<small>达到阈值时，在对话中询问是否压缩。</small></span><input type="checkbox" class="switch" v-model="draft.compaction.enabled" @change="!draft.compaction.enabled && (draft.compaction.automatic = false)" /></label>
        <label class="row"><span>自动压缩上下文<small>达到阈值时直接压缩，随后继续本轮对话。</small></span><input type="checkbox" class="switch" v-model="draft.compaction.automatic" :disabled="!draft.compaction.enabled" /></label>
        <label class="row"><span>模型上下文窗口比例</span><span class="number"><input type="number" required min="1" max="100" step="1" v-model.number="draft.compaction.windowPercent" /> %</span></label>
        <label class="row"><span>固定 token 阈值</span><input type="number" required min="1" max="2147483647" step="1" v-model.number="draft.compaction.tokenThreshold" /></label>
        <p>先取两者中较小的值。若选择暂不压缩，到达较大值时再提醒一次。模型窗口未知时，只使用固定阈值。</p>
      </fieldset>
      <footer><span role="status">{{ dirty ? '有未保存的更改' : saved }}</span><button class="btn btn-primary" type="submit" :disabled="busy || !valid || !dirty">{{ busy ? '正在保存…' : '保存设置' }}</button></footer>
    </form>
  </div></section>
</template>

<style scoped>
.page { flex: 1; min-height: 0; overflow: auto; padding: 26px clamp(16px, 4vw, 36px); }
.content { max-width: 680px; margin: 0 auto; } h1 { font-size: 21px; font-weight: 600; }
p, small, footer span { color: var(--muted); font-size: 12px; line-height: 1.6; } header p { margin: 4px 0 18px; }
fieldset { min-width: 0; margin: 0 0 14px; padding: 8px 18px 12px; border: 1px solid var(--outline); border-radius: 14px; background: var(--surface); }
legend { padding: 0 5px; font-size: 14px; font-weight: 600; } .row { display: flex; align-items: center; justify-content: space-between; gap: 14px; padding: 10px 0; border-bottom: 1px solid var(--outline); font-size: 13px; } .row:last-child { border: 0; }
.row small { display: block; }.nested { margin-left: 12px; } summary { padding: 9px 0; cursor: pointer; font-size: 13px; } summary small { margin-left: 5px; }
input[type=number], select { width: 112px; border: 1px solid var(--outline); background: var(--surface); border-radius: 6px; padding: 5px 7px; font: inherit; color: var(--ink); }
.number { white-space: nowrap; } input[type=checkbox] { flex-shrink: 0; accent-color: var(--accent); } footer { display: flex; align-items: center; justify-content: flex-end; gap: 12px; } .error { color: var(--danger); }
</style>
