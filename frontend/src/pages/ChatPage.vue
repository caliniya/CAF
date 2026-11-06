<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { onChatDelta, onChatTool, ListToolSets, SetChatToolConfig, type Persona, type ToolSetDescriptor } from '../api'
import { useChatStore } from '../stores/chat'
import { useConfigsStore } from '../stores/configs'
import { ArrowUp, Box, Check, ChevronDown, ChevronRight, Copy, Folder, Info, Pencil, Plus, RotateCw, Settings2, Shield, UserRound } from '@lucide/vue'
import PersonaDetailsDialog from '../components/PersonaDetailsDialog.vue'
import { renderDone, renderStreaming, resetStreaming } from '../markdown'

const chat = useChatStore()
const configs = useConfigsStore()
const scroller = ref<HTMLElement | null>(null)
const modelMenuAnchor = ref<HTMLElement | null>(null)
const modelMenuOpen = ref(false)

// ===== 权限菜单 =====
const PERMISSIONS = ['只读', '标准', '完全访问'] as const
const permMenuAnchor = ref<HTMLElement | null>(null)
const permMenuOpen = ref(false)
const permFlyoutOpen = ref(false)
const permFlyoutFlip = ref(false)
const toolsOpen = ref(false)

// ===== 人设 =====
const personaMenuOpen = ref(false)
const personaAnchor = ref<HTMLElement | null>(null)
const personaLocked = computed(() => !!chat.session)
const currentPersonaId = computed(() =>
  (chat.session ? chat.session.tree?.root?.personaId || 'assistant' : chat.pendingPersonaId)
  || chat.personas[0]?.id || '',
)
const currentPersona = computed(() => chat.personas.find((item) => item.id === currentPersonaId.value))

// ===== 对话设置 =====
const settingsMenuAnchor = ref<HTMLElement | null>(null)
const settingsMenuButton = ref<HTMLButtonElement | null>(null)
const settingsMenuOpen = ref(false)
const settingsPersonaOpen = ref(false)
const settingsPersonaFlyout = ref<HTMLElement | null>(null)
const settingsPersonaFlip = ref(false)
const settingsPersonaTop = ref(0)
const detailsPersona = ref<Persona | null>(null)

function pickPersona(id: string) {
  if (personaLocked.value) return
  chat.setPendingPersona(id)
  closeMenus()
}

function closeMenus() {
  modelMenuOpen.value = false
  openFlyout.value = ''
  permMenuOpen.value = false
  permFlyoutOpen.value = false
  personaMenuOpen.value = false
  settingsMenuOpen.value = false
  settingsPersonaOpen.value = false
}

// 一级菜单互斥，避免同时弹开
function toggleModelMenu() {
  const open = !modelMenuOpen.value
  closeMenus()
  modelMenuOpen.value = open
}

function togglePermMenu() {
  const open = !permMenuOpen.value
  closeMenus()
  permMenuOpen.value = open
}

function togglePersonaMenu() {
  const open = !personaMenuOpen.value
  closeMenus()
  personaMenuOpen.value = open
}

function toggleSettingsMenu() {
  const open = !settingsMenuOpen.value
  closeMenus()
  settingsMenuOpen.value = open
}

async function toggleSettingsPersona(event: MouseEvent) {
  if (settingsPersonaOpen.value) {
    settingsPersonaOpen.value = false
    return
  }
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  settingsPersonaFlip.value = bounds.right + 256 > window.innerWidth - 8
  settingsPersonaTop.value = 0
  settingsPersonaOpen.value = true
  await nextTick()
  const flyoutBounds = settingsPersonaFlyout.value?.getBoundingClientRect()
  if (flyoutBounds) {
    settingsPersonaTop.value = -Math.max(0, flyoutBounds.bottom - window.innerHeight + 8)
  }
}

function togglePermFlyout(event: MouseEvent) {
  if (permFlyoutOpen.value) {
    permFlyoutOpen.value = false
    return
  }
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  permFlyoutFlip.value = bounds.right + 148 > window.innerWidth - 8
  permFlyoutOpen.value = true
}

function pickPermission(level: string) {
  chat.permission = level
  permFlyoutOpen.value = false
  permMenuOpen.value = false
}

const toolSets = ref<ToolSetDescriptor[]>([])
const toolOverrides = ref<Record<string, boolean>>({})
const toolsLoading = ref(false)
const expandedSets = ref<Record<string, boolean>>({})

// 用 \0 作分隔符，工具集/工具名里不可能出现
function toolKey(setID: string, kitID: string) {
  return `${setID}\u0000${kitID}`
}

function readOverrides() {
  // 有会话读会话里的；没有就读「对话开始前」攒下的配置
  const sets = chat.session?.toolConfig?.sets ?? chat.pendingToolConfig?.sets ?? {}
  const result: Record<string, boolean> = {}
  for (const [setID, info] of Object.entries(sets)) {
    for (const [kitID, on] of Object.entries(info.kits ?? {})) {
      result[toolKey(setID, kitID)] = on
    }
  }
  toolOverrides.value = result
}

async function openTools() {
  permMenuOpen.value = false
  permFlyoutOpen.value = false
  toolsOpen.value = true
  toolsLoading.value = true
  try {
    toolSets.value = await ListToolSets()
    readOverrides()
    const expanded: Record<string, boolean> = {}
    for (const set of toolSets.value) expanded[set.id] = true
    expandedSets.value = expanded
  } catch (err) {
    chat.error = String(err)
    toolSets.value = []
    toolOverrides.value = {}
  } finally {
    toolsLoading.value = false
  }
}

function toggleSet(setID: string) {
  expandedSets.value = { ...expandedSets.value, [setID]: !expandedSets.value[setID] }
}

function toolEnabled(setID: string, kitID: string, fallback: boolean) {
  const key = toolKey(setID, kitID)
  return key in toolOverrides.value ? toolOverrides.value[key] : fallback
}

// 整个工具集的开关状态：全开 / 全关 / 混合
function setState(set: ToolSetDescriptor): 'on' | 'off' | 'mixed' {
  if (!set.kits.length) return 'off'
  const flags = set.kits.map((kit) => toolEnabled(set.id, kit.id, kit.defaultEnabled))
  if (flags.every(Boolean)) return 'on'
  if (flags.every((value) => !value)) return 'off'
  return 'mixed'
}

// 工具集总开关 = 批量写入它下面所有 ToolKit，落地仍是 kit 粒度
function setAllKits(set: ToolSetDescriptor, on: boolean) {
  const next = { ...toolOverrides.value }
  for (const kit of set.kits) next[toolKey(set.id, kit.id)] = on
  void saveOverrides(next)
}

async function saveOverrides(next: Record<string, boolean>) {
  toolOverrides.value = next
  const sets: Record<string, { kits: Record<string, boolean> }> = {}
  for (const [key, on] of Object.entries(next)) {
    const [setID, kitID] = key.split('\u0000')
    if (!setID || !kitID) continue
    if (!sets[setID]) sets[setID] = { kits: {} }
    sets[setID].kits[kitID] = on
  }
  const config = Object.keys(next).length ? { sets } : null
  const id = chat.session?.id
  if (!id) {
    // 对话还没创建：先攒着，createChat 成功后写进新会话
    chat.setPendingToolConfig(config)
    return
  }
  try {
    const session = await SetChatToolConfig(id, JSON.stringify(config ?? {}))
    if (chat.session?.id === id) chat.session = session
  } catch (err) {
    chat.error = String(err)
  }
}

function setToolKit(setID: string, kitID: string, on: boolean) {
  void saveOverrides({ ...toolOverrides.value, [toolKey(setID, kitID)]: on })
}

function resetToolConfig() {
  void saveOverrides({})
}

// 同一时刻只允许一个二级菜单展开：provider = 服务商，model = 模型，effort = 思考强度
const openFlyout = ref<'' | 'provider' | 'model' | 'effort'>('')
// 二级菜单往右展开会超出窗口时，翻到左侧
const flyoutFlip = ref(false)

function toggleFlyout(name: 'provider' | 'model' | 'effort', event: MouseEvent) {
  if (openFlyout.value === name) {
    openFlyout.value = ''
    return
  }
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const flyoutWidth = name === 'effort' ? 148 : 232
  flyoutFlip.value = bounds.right + flyoutWidth > window.innerWidth - 8
  openFlyout.value = name
}

function pickEffort(index: number) {
  effortIndex.value = index
  openFlyout.value = ''
}

// Ctrl+T 循环切换思考强度
function onGlobalKeydown(event: KeyboardEvent) {
  if (detailsPersona.value) return
  if (event.key === 'Escape') {
    const settingsWasOpen = settingsMenuOpen.value
    closeMenus()
    if (settingsWasOpen) settingsMenuButton.value?.focus()
    return
  }
  if (!event.ctrlKey || event.key.toLowerCase() !== 't') return
  event.preventDefault()
  effortIndex.value = (effortIndex.value + 1) % effortSteps.length
}

const activeConfig = computed(
  () => configs.configs.find((item) => item.ID === chat.session?.configId) ?? configs.enables,
)
const activeModel = computed(() => chat.session?.model || activeConfig.value?.EnabledModel || '')

// 思考强度：四档，与后端 SendChatMessage 的 reasoningEffort 参数对应。
const effortSteps = [
  { value: 'none', label: '关闭' },
  { value: 'low', label: '低' },
  { value: 'high', label: '中' },
  { value: 'max', label: '高' },
] as const

const effortIndex = computed({
  get: () => {
    const index = effortSteps.findIndex((step) => step.value === chat.effort)
    // 找不到时回落到「中」，而不是「关闭」
    return index < 0 ? 2 : index
  },
  set: (index: number) => {
    const step = effortSteps[Math.round(index)]
    if (step) chat.setEffort(step.value)
  },
})

const effortLabel = computed(() => effortSteps[effortIndex.value].label)

// 当前模型在菜单里显示的名字（优先显示 provider 给的展示名）
const activeModelName = computed(() => {
  const found = activeConfig.value?.Model?.find((item) => item.id === activeModel.value)
  return found?.name || activeModel.value || '未选择'
})

let offs: (() => void)[] = []

async function selectConfig(configId: string) {
  const config = configs.configs.find((item) => item.ID === configId)
  const model = config?.EnabledModel || config?.Model[0]?.id
  if (!config) return
  try {
    if (chat.session) {
      if (!model) return
      await chat.updateModel(configId, model)
    } else {
      await configs.selectConfig(configs.configs.indexOf(config))
    }
    openFlyout.value = ''
  } catch (err) {
    chat.error = String(err)
  }
}

async function selectModel(model: string) {
  if (!model || !activeConfig.value) return
  try {
    if (chat.session) {
      await chat.updateModel(chat.session.configId, model)
    } else {
      await configs.selectModel(configs.configs.indexOf(activeConfig.value), model)
    }
    openFlyout.value = ''
    modelMenuOpen.value = false
  } catch (err) {
    chat.error = String(err)
  }
}

function closeMenusOnOutside(event: PointerEvent) {
  if (detailsPersona.value) return
  const target = event.target as Node
  if (modelMenuAnchor.value?.contains(target) || permMenuAnchor.value?.contains(target)
    || personaAnchor.value?.contains(target) || settingsMenuAnchor.value?.contains(target)) return
  closeMenus()
}

// Enter 发送；带修饰键的 Enter（Shift、Ctrl+Shift 等）统一插入换行。
// 浏览器对 Ctrl+Shift+Enter 默认没有行为，所以这里手动插入，不依赖默认行为。
function onComposerKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.isComposing || event.keyCode === 229) return
  const modifier = event.shiftKey || event.ctrlKey || event.altKey || event.metaKey
  if (!modifier) {
    event.preventDefault()
    chat.send()
    return
  }
  event.preventDefault()
  const el = event.currentTarget as HTMLTextAreaElement
  const start = el.selectionStart
  const end = el.selectionEnd
  chat.draft = chat.draft.slice(0, start) + '\n' + chat.draft.slice(end)
  nextTick(() => {
    el.selectionStart = el.selectionEnd = start + 1
  })
}

async function selectBranch(message: (typeof chat.messages)[number], direction: number) {
  if (!message.parentId || message.branchIndex === undefined || !message.branchCount) return
  const next = (message.branchIndex + direction + message.branchCount) % message.branchCount
  try {
    await chat.selectBranch(message.parentId, next)
  } catch (err) {
    chat.error = String(err)
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', closeMenusOnOutside)
  document.addEventListener('keydown', onGlobalKeydown)
  onChatDelta((delta) => chat.applyDelta(delta)).then((off) => offs.push(off))
  onChatTool((tool) => chat.applyTool(tool)).then((off) => offs.push(off))
  if (!configs.loaded) configs.load()
  chat.loadPersonas().catch((err) => { chat.error = String(err) })
  if (!chat.chats.length) chat.loadChats().catch((err) => { chat.error = String(err) })
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', closeMenusOnOutside)
  document.removeEventListener('keydown', onGlobalKeydown)
  offs.forEach((off) => off())
})

// 一段新的流式输出开始时清掉上一帧缓存
watch(() => chat.sending, (sending) => {
  if (sending) resetStreaming()
})

// 代码块复制：v-html 生成的按钮无法直接绑 Vue 事件，用事件委托处理
async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // 自定义协议下 clipboard API 可能不可用，走下面的兜底
  }
  const area = document.createElement('textarea')
  area.value = text
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.select()
  const ok = document.execCommand('copy')
  document.body.removeChild(area)
  return ok
}

async function onBodyClick(event: MouseEvent) {
  if (!(event.target instanceof Element)) return
  const button = event.target.closest<HTMLButtonElement>('.code-copy')
  if (!button) return
  const code = button.closest('.code-block')?.querySelector('pre code')?.textContent ?? ''
  if (!code) return
  const ok = await copyText(code)
  button.textContent = ok ? '已复制' : '复制失败'
  button.classList.toggle('copied', ok)
  window.setTimeout(() => {
    button.textContent = '复制'
    button.classList.remove('copied')
  }, 1600)
}

// ===== 编辑用户消息 =====
const editingNodeId = ref('')
const editDraft = ref('')

async function startEdit(message: (typeof chat.messages)[number]) {
  if (!message.nodeId || chat.sending) return
  editingNodeId.value = message.nodeId
  editDraft.value = message.content
  // 等 DOM 换成 textarea 后立刻聚焦，否则要点第二次才能输入
  await nextTick()
  const input = document.querySelector<HTMLTextAreaElement>('.msg-edit-input')
  if (input) {
    input.focus()
    input.setSelectionRange(input.value.length, input.value.length)
  }
}

function cancelEdit() {
  editingNodeId.value = ''
  editDraft.value = ''
}

async function submitEdit(message: (typeof chat.messages)[number]) {
  const text = editDraft.value.trim()
  if (!text || text === message.content) {
    cancelEdit()
    return
  }
  await chat.editMessage(message, text)
  cancelEdit()
}

async function copyMessage(message: (typeof chat.messages)[number], event: MouseEvent) {
  // currentTarget 在 await 之后会被浏览器清空，先存引用
  const button = event.currentTarget as HTMLElement
  const ok = await copyText(message.content)
  button.classList.toggle('copied', ok)
  window.setTimeout(() => button.classList.remove('copied'), 1600)
}

// 用户消息保持纯文本；助手消息渲染 Markdown。
// 只有正在生成的最后一条走流式解析，其余全部读缓存。
function bodyHtml(message: (typeof chat.messages)[number], index: number): string {
  const streaming =
    chat.sending && message.role === 'assistant' && index === chat.messages.length - 1
  if (streaming) {
    return renderStreaming(message.content) + '<span class="cursor" aria-hidden="true">▍</span>'
  }
  return renderDone(`${message.nodeId ?? 'draft'}:${index}`, message.content)
}

watch(
  () => [chat.session?.id, chat.session?.configId, chat.session?.model],
  () => chat.loadContextWindow(),
  { immediate: true },
)

// 消息条数、正文/思考/工具卡变化(流式增量)时滚到底部。
watch(
  () => {
    const last = chat.messages[chat.messages.length - 1]
    return `${chat.messages.length}:${last?.content.length ?? 0}:${last?.process?.length ?? 0}:${chat.error.length}`
  },
  async () => {
    await nextTick()
    const el = scroller.value
    if (el) el.scrollTop = el.scrollHeight
  },
)
</script>

<template>
  <section class="chat">
    <div ref="scroller" class="chat-body" @click="onBodyClick">
      <div class="chat-list">
      <div v-for="(m, i) in chat.messages" :key="`${m.nodeId}:${i}`" class="msg" :class="m.role">
        <!-- 模型工作过程：思考 + 中间轮次的文本 + 工具调用，最终回复在外面 -->
        <details v-if="m.role === 'assistant' && m.process?.length" class="process"
          :open="chat.sending && i === chat.messages.length - 1">
          <summary>模型工作过程</summary>
          <div class="process-body">
            <template v-for="(item, pi) in m.process" :key="pi">
              <!-- 每轮思考单独可折叠；生成中自动展开，结束后交给用户控制 -->
              <details v-if="item.kind === 'reasoning'" class="process-reason"
                :open="chat.sending && i === chat.messages.length - 1 ? true : undefined">
                <summary>思考</summary>
                <p class="process-reasoning">{{ item.content }}</p>
              </details>
              <p v-else-if="item.kind === 'text'" class="process-text">{{ item.content }}</p>
              <div v-else class="process-tool">
                <span class="process-dot" :class="item.status" aria-hidden="true"></span>
                执行 {{ item.name }}
              </div>
            </template>
          </div>
        </details>

        <!-- 编辑中的用户消息 -->
        <template v-if="m.role === 'user' && m.nodeId && editingNodeId === m.nodeId">
          <textarea v-model="editDraft" class="msg-edit-input" rows="2"
            @keydown.esc.prevent="cancelEdit" @keydown.enter.exact.prevent="submitEdit(m)"></textarea>
          <div class="msg-edit-actions">
            <button type="button" class="btn btn-ghost btn-sm" @click="cancelEdit">取消</button>
            <button type="button" class="btn btn-primary btn-sm" :disabled="!editDraft.trim()"
              @click="submitEdit(m)">保存并重新生成</button>
          </div>
        </template>
        <template v-else>
          <div v-if="m.role === 'user'" class="msg-body">{{ m.content }}</div>
          <div v-else class="msg-body markdown-body" v-html="bodyHtml(m, i)"></div>
        </template>

        <!-- 操作栏：编辑 / 复制 / 重新生成 / 分支切换统一一行，鼠标靠近才显示 -->
        <div v-if="editingNodeId !== m.nodeId" class="msg-actions">
          <button v-if="m.role === 'user' && m.nodeId && !chat.sending" type="button"
            class="msg-action hover-only" title="编辑" aria-label="编辑" @click="startEdit(m)">
            <Pencil :size="17" :stroke-width="2" />
          </button>
          <button v-if="m.content" type="button" class="msg-action hover-only" title="复制"
            aria-label="复制" @click="copyMessage(m, $event)">
            <Copy :size="17" :stroke-width="2" />
          </button>
          <button v-if="m.role === 'assistant' && m.nodeId && !chat.sending" type="button"
            class="regen hover-only" title="重新生成" aria-label="重新生成" @click="chat.regenerate(m)">
            <RotateCw :size="17" :stroke-width="2" />
          </button>
          <div v-if="m.branchCount && m.branchCount > 1" class="branch hover-only">
            <button type="button" aria-label="上一个分支" @click="selectBranch(m, -1)">‹</button>
            <span>{{ (m.branchIndex ?? 0) + 1 }} / {{ m.branchCount }}</span>
            <button type="button" aria-label="下一个分支" @click="selectBranch(m, 1)">›</button>
          </div>
        </div>
      </div>
      <p v-if="chat.error" class="chat-error">{{ chat.error }}</p>
      </div>
    </div>

    <div class="composer-wrap">
      <!-- 工作区条：对话为空时才出现，视觉上压在输入框后面 -->
      <div class="composer-frame">
        <div class="workspace-bar">
          <button type="button" class="ctx-pill" title="切换工作区">
            <Folder :size="14" :stroke-width="2" />
            <span>CAF</span>
            <ChevronDown class="chev" :size="12" :stroke-width="2" />
          </button>

          <!-- 人设：建对话时确定，之后锁定 -->
          <div ref="personaAnchor" class="persona-anchor">
            <button type="button" class="persona-btn" :class="{ locked: personaLocked }"
              :disabled="personaLocked"
              :title="personaLocked ? '人设在对话开始时确定，新建对话可切换' : '选择本次对话的人设'"
              @click="togglePersonaMenu">
              <UserRound :size="13" :stroke-width="2" />
              <span>{{ currentPersona?.name ?? '人设' }}</span>
              <ChevronDown v-if="!personaLocked" class="chev" :size="11" :stroke-width="2" />
            </button>

            <div v-if="personaMenuOpen && !personaLocked" class="menu persona-menu" role="menu">
              <button v-for="item in chat.personas" :key="item.id" type="button" class="menu-row"
                :class="{ on: item.id === currentPersonaId }" :title="item.description"
                @click="pickPersona(item.id)">
                <span class="menu-row-name">{{ item.name }}</span>
                <span class="menu-spacer"></span>
                <Check v-if="item.id === currentPersonaId" class="check" :size="14" :stroke-width="2" />
              </button>
            </div>
          </div>
        </div>

        <div class="composer">
          <textarea v-model="chat.draft" class="composer-input" rows="1"
            placeholder="Enter 发送 · Shift+Enter / Ctrl+Shift+Enter 换行"
            @keydown="onComposerKeydown"></textarea>

          <div class="composer-bar">
            <button type="button" class="bar-icon" title="附件" aria-label="附件">
              <Plus :size="16" :stroke-width="2" />
            </button>

            <div ref="permMenuAnchor" class="perm-anchor">
              <button type="button" class="perm-trigger" :class="{ elevated: chat.permission === '完全访问' }"
                :aria-expanded="permMenuOpen" aria-haspopup="menu" @click="togglePermMenu">
                <Shield :size="13" :stroke-width="2" />
                <span>{{ chat.permission }}</span>
                <ChevronDown class="chev" :size="12" :stroke-width="2" />
              </button>

              <div v-if="permMenuOpen" class="menu perm-menu" role="menu">
                <div class="menu-slot">
                  <button type="button" class="menu-row" :aria-expanded="permFlyoutOpen"
                    @click="togglePermFlyout">
                    <span class="menu-row-label">权限</span>
                    <span class="menu-spacer"></span>
                    <span class="menu-value">{{ chat.permission }}</span>
                    <ChevronRight class="chev" :size="12" :stroke-width="2" />
                  </button>

                  <div v-if="permFlyoutOpen" class="flyout flyout--narrow"
                    :class="{ 'flyout--flip': permFlyoutFlip }" role="menu">
                    <button v-for="level in PERMISSIONS" :key="level" type="button" class="menu-row"
                      :class="{ on: chat.permission === level }" @click="pickPermission(level)">
                      <span class="menu-row-name">{{ level }}</span>
              <Check v-if="chat.permission === level" class="check" :size="14" :stroke-width="2" />
                    </button>
                  </div>
                </div>

                <div class="menu-sep"></div>
                <button type="button" class="menu-footer" @click="openTools">工具配置</button>
              </div>
            </div>

            <span class="bar-spacer"></span>

            <!-- 上下文占用：仅在有对话内容时出现，悬停看完整数值 -->
            <span v-if="chat.messages.length" class="ctx" :data-tooltip="chat.ctxTooltip" :aria-label="chat.ctxTooltip"
              tabindex="0">
              <svg class="ctx-ring" viewBox="0 0 20 20" aria-hidden="true">
                <circle class="ctx-ring-bg" cx="10" cy="10" r="8" />
                <circle class="ctx-ring-fill" :class="{ unknown: chat.ctxPercent === null }" cx="10" cy="10" r="8"
                  :style="{ strokeDashoffset: 50.27 * (1 - (chat.ctxPercent ?? 0) / 100) }" />
              </svg>
            </span>

            <div ref="settingsMenuAnchor" class="settings-anchor">
              <button ref="settingsMenuButton" type="button" class="bar-icon chat-settings-btn"
                title="对话设置" aria-label="对话设置" :aria-expanded="settingsMenuOpen"
                aria-haspopup="menu" aria-controls="chat-settings-menu" @click="toggleSettingsMenu">
                <Settings2 :size="16" :stroke-width="2" />
              </button>

              <div v-if="settingsMenuOpen" id="chat-settings-menu" class="menu settings-menu"
                role="menu" aria-label="对话设置">
                <div class="menu-slot">
                  <button type="button" class="menu-row" role="menuitem" :aria-expanded="settingsPersonaOpen"
                    aria-haspopup="menu" aria-controls="chat-persona-menu" @click="toggleSettingsPersona">
                    <UserRound class="row-icon" :size="16" :stroke-width="1.6" />
                    <span class="menu-row-label">角色设定</span>
                    <span class="menu-row-name settings-persona-name">{{ currentPersona?.name ?? '未选择人设' }}</span>
                    <ChevronRight class="chev" :size="12" :stroke-width="2" />
                  </button>

                  <div v-if="settingsPersonaOpen" id="chat-persona-menu" ref="settingsPersonaFlyout"
                    class="flyout settings-persona-flyout" :class="{ 'flyout--flip': settingsPersonaFlip }"
                    :style="{ top: `${settingsPersonaTop}px` }" role="menu" aria-label="角色设定">
                    <div v-for="item in chat.personas" :key="item.id" class="persona-option"
                      :class="{ on: item.id === currentPersonaId }" role="none">
                      <button type="button" class="persona-option-select" role="menuitemradio"
                        :aria-checked="item.id === currentPersonaId" :disabled="personaLocked"
                        :title="personaLocked ? '人设在对话开始时确定，新建对话可切换' : item.description"
                        @click="pickPersona(item.id)">
                        <span class="menu-row-name">{{ item.name }}</span>
                        <Check v-if="item.id === currentPersonaId" class="check" :size="14" :stroke-width="2" />
                      </button>
                      <button type="button" class="persona-detail-btn" role="menuitem"
                        title="查看人设详情" :aria-label="`查看「${item.name}」的人设详情`"
                        @click.stop="detailsPersona = item">
                        <Info :size="15" :stroke-width="2" />
                      </button>
                    </div>
                    <span v-if="!chat.personas.length" class="menu-empty">暂无人设</span>
                  </div>
                </div>
              </div>
            </div>

            <div ref="modelMenuAnchor" class="model-anchor">
              <button type="button" class="model-btn" :aria-expanded="modelMenuOpen" aria-haspopup="menu"
                @click="toggleModelMenu">
                <span class="model-btn-label">
                  {{ activeModel || '选择模型' }}{{ activeModel && chat.effort !== 'none' ? ` ${effortLabel}` : '' }}
                </span>
                <ChevronDown class="chev" :size="12" :stroke-width="2" />
              </button>

              <div v-if="modelMenuOpen" class="menu model-menu" role="menu">
                <!-- 提供商 -->
                <div class="menu-slot">
                  <button type="button" class="menu-row" :aria-expanded="openFlyout === 'provider'"
                    @click="toggleFlyout('provider', $event)">
                    <Box class="row-icon" :size="16" :stroke-width="1.6" />
                    <span class="menu-row-name">
                      {{ activeConfig?.Name || activeConfig?.ServerAddress || '未选择提供商' }}
                    </span>
                    <span class="tag">API</span>
                    <ChevronRight class="chev" :size="12" :stroke-width="2" />
                  </button>

                  <div v-if="openFlyout === 'provider'" class="flyout" :class="{ 'flyout--flip': flyoutFlip }"
                    role="menu">
                    <button v-for="config in configs.configs" :key="config.ID" type="button" class="menu-row"
                      :class="{ on: config.ID === activeConfig?.ID }" @click="selectConfig(config.ID)">
                      <Box class="row-icon" :size="16" :stroke-width="1.6" />
                      <span class="menu-row-name">{{ config.Name || config.ServerAddress }}</span>
                      <span class="tag">API</span>
                      <span class="menu-spacer"></span>
              <Check v-if="config.ID === activeConfig?.ID" class="check" :size="14" :stroke-width="2" />
                    </button>
                    <span v-if="!configs.configs.length" class="menu-empty">尚未配置提供商</span>
                    <div class="menu-sep"></div>
                    <RouterLink class="menu-row" to="/settings/models" @click="modelMenuOpen = false">
                      管理模型
                    </RouterLink>
                  </div>
                </div>

                <!-- 模型：点开二级菜单选 -->
                <div class="menu-slot">
                  <button type="button" class="menu-row" :aria-expanded="openFlyout === 'model'"
                    @click="toggleFlyout('model', $event)">
                    <span class="menu-row-label">模型</span>
                    <span class="menu-spacer"></span>
                    <span class="menu-value">{{ activeModelName }}</span>
                    <ChevronRight class="chev" :size="12" :stroke-width="2" />
                  </button>

                  <div v-if="openFlyout === 'model'" class="flyout" :class="{ 'flyout--flip': flyoutFlip }" role="menu">
                    <button v-for="model in activeConfig?.Model ?? []" :key="model.id" type="button" class="menu-item"
                      :class="{ on: model.id === activeModel }" @click="selectModel(model.id)">
                      <span class="menu-item-name">{{ model.name || model.id }}</span>
                      <span v-if="model.input_modalities?.includes('image')" class="tag">视觉</span>
              <Check v-if="model.id === activeModel" class="check" :size="14" :stroke-width="2" />
                    </button>
                    <span v-if="!activeConfig?.Model?.length" class="menu-empty">暂无模型</span>
                  </div>
                </div>

                <!-- 思考强度：点开二级菜单选档位 -->
                <div class="menu-slot">
                  <button type="button" class="menu-row" :aria-expanded="openFlyout === 'effort'"
                    @click="toggleFlyout('effort', $event)">
                    <span class="menu-row-label">思考强度</span>
                    <span class="menu-spacer"></span>
                    <span class="menu-value">{{ effortLabel }}</span>
                    <kbd class="menu-kbd">Ctrl+T</kbd>
                    <ChevronRight class="chev" :size="12" :stroke-width="2" />
                  </button>

                  <div v-if="openFlyout === 'effort'" class="flyout flyout--narrow"
                    :class="{ 'flyout--flip': flyoutFlip }" role="menu">
                    <button v-for="(step, index) in effortSteps" :key="step.value" type="button" class="menu-row"
                      :class="{ on: index === effortIndex }" @click="pickEffort(index)">
                      <span class="menu-row-name">{{ step.label }}</span>
              <Check v-if="index === effortIndex" class="check" :size="14" :stroke-width="2" />
                    </button>
                  </div>
                </div>

                <div class="menu-sep"></div>

                <RouterLink class="menu-row" to="/settings/models" @click="modelMenuOpen = false">
                  模型配置
                </RouterLink>
              </div>
            </div>

            <button type="button" class="send" :disabled="!chat.draft.trim() || chat.sending" aria-label="发送"
              @click="chat.send()">
              <ArrowUp :size="17" :stroke-width="2.2" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </section>

  <PersonaDetailsDialog :persona="detailsPersona" @close="detailsPersona = null" />

  <!-- 当前对话的工具设置（占位） -->
  <Teleport to="body">
    <div v-if="toolsOpen" class="overlay" @click.self="toolsOpen = false">
      <div class="tools-dialog" role="dialog" aria-modal="true">
        <header class="tools-head">
          <div>
            <h2>工具配置</h2>
            <p class="tools-sub">
              {{ chat.session ? '改动只作用于当前对话' : '为即将开始的对话预设工具' }}
            </p>
          </div>
          <button type="button" class="btn btn-ghost btn-sm" @click="toolsOpen = false">关闭</button>
        </header>

        <div class="tools-body">
          <p v-if="toolsLoading" class="tools-empty">正在加载工具…</p>
          <p v-else-if="!toolSets.length" class="tools-empty">没有加载到工具集</p>

          <div v-else class="tool-groups">
            <section v-for="set in toolSets" :key="set.id" class="tool-group">
              <div class="tool-group-head">
                <button type="button" class="tool-expand" :aria-expanded="!!expandedSets[set.id]"
                  @click="toggleSet(set.id)">
                  <ChevronDown class="chev" :class="{ collapsed: !expandedSets[set.id] }" :size="13" :stroke-width="2" />
                  <span class="tool-set-name">{{ set.name }}</span>
                  <span class="tool-count">{{ set.kits.length }} 个模块</span>
                </button>
                <input type="checkbox" class="switch" :checked="setState(set) !== 'off'"
                  :indeterminate.prop="setState(set) === 'mixed'" :aria-label="`${set.name} 全部模块`"
                  @change="setAllKits(set, ($event.target as HTMLInputElement).checked)" />
              </div>
              <p class="tool-desc">{{ set.description }}</p>

              <!-- 展开后才显示子模块；不列出模块里有哪些工具 -->
              <div v-if="expandedSets[set.id]" class="tool-kits">
                <label v-for="kit in set.kits" :key="kit.id" class="tool-row">
                  <span class="tool-text">
                    <span class="tool-name">{{ kit.name }}</span>
                    <span class="tool-desc">{{ kit.description }}</span>
                  </span>
                  <input type="checkbox" class="switch"
                    :checked="toolEnabled(set.id, kit.id, kit.defaultEnabled)" :aria-label="kit.name"
                    @change="setToolKit(set.id, kit.id, ($event.target as HTMLInputElement).checked)" />
                </label>
              </div>
            </section>
          </div>
        </div>

        <footer class="tools-foot">
          <button type="button" class="btn btn-ghost btn-sm" @click="resetToolConfig">恢复默认</button>
          <button type="button" class="btn btn-primary btn-sm" @click="toolsOpen = false">完成</button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.chat {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* 消息区：唯一滚动容器 */
/* 滚动容器铺满宽度，滚动条贴到窗口右侧；内容宽度由内层限制 */
.chat-body {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  width: 100%;
  overflow-y: auto;
}

.chat-body::-webkit-scrollbar {
  width: 10px;
}

.chat-body::-webkit-scrollbar-thumb {
  border: 3px solid transparent;
  border-radius: 999px;
  background: color-mix(in srgb, var(--muted) 32%, transparent);
  background-clip: content-box;
}

.chat-body::-webkit-scrollbar-thumb:hover {
  background: color-mix(in srgb, var(--muted) 55%, transparent);
  background-clip: content-box;
}

.chat-list {
  display: flex;
  flex-direction: column;
  gap: 18px;
  width: 100%;
  max-width: 1100px;
  margin: 0 auto;
  padding: 24px 28px 48px;
}

/* ===== 消息 ===== */
.msg {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: flex-start;
}

.msg.user {
  align-items: flex-end;
}

.msg-body {
  max-width: 100%;
  overflow-wrap: anywhere;
}

/* 只有用户消息需要保留原文换行；助手消息交给 markdown.css */
.msg.user .msg-body {
  white-space: pre-wrap;
  max-width: 78%;
  padding: 9px 14px;
  border-radius: 12px;
  background: var(--tonal);
  color: var(--ink);
}

/* ===== 消息操作栏 ===== */
.msg-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  min-height: 32px;
}

/* 分支切换与其它按钮同排，间距略大一点 */
.msg-actions .branch {
  margin-left: 2px;
}

/* 编辑、复制只在鼠标靠近消息时出现 */
.msg-actions .hover-only {
  opacity: 0;
  pointer-events: none;
  transition: opacity 120ms ease;
}

.msg:hover .msg-actions .hover-only,
.msg-actions .hover-only:focus-visible {
  opacity: 1;
  pointer-events: auto;
}

.msg-edit-input {
  width: min(520px, 100%);
  min-height: 72px;
  max-height: 260px;
  padding: 10px 12px;
  border: 1px solid var(--accent);
  border-radius: 10px;
  background: var(--surface);
  color: var(--ink);
  font-size: 14px;
  line-height: 1.6;
  resize: vertical;
  /* 高度跟随内容，减少切换编辑态时的跳动 */
  field-sizing: content;
  animation: edit-in 120ms ease;
}

@keyframes edit-in {
  from {
    opacity: 0;
    transform: translateY(-2px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}

.msg-edit-input:focus {
  outline: none;
  box-shadow: 0 0 0 3px var(--tonal);
}

.msg-edit-actions {
  display: flex;
  gap: 8px;
}

/* ===== 模型工作过程 ===== */
.process {
  width: 100%;
  border: 1px solid var(--outline);
  border-radius: 10px;
  background: var(--page);
  font-size: 12.5px;
}

.process summary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  color: var(--muted);
  cursor: pointer;
  list-style: none;
  user-select: none;
}

.process summary::-webkit-details-marker {
  display: none;
}

.process summary::before {
  content: '';
  width: 0;
  height: 0;
  border-left: 4px solid currentColor;
  border-top: 3.5px solid transparent;
  border-bottom: 3.5px solid transparent;
  transition: transform 150ms ease;
}

.process[open] summary::before {
  transform: rotate(90deg);
}

.process-body {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 0 12px 12px;
}

/* 单轮思考：可折叠，默认收起 */
.process-reason summary {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--muted);
  font-size: 12px;
  cursor: pointer;
  list-style: none;
  user-select: none;
}

.process-reason summary::-webkit-details-marker {
  display: none;
}

.process-reason summary::before {
  content: '▸';
  font-size: 10px;
  transition: transform 150ms ease;
}

.process-reason[open] summary::before {
  transform: rotate(90deg);
}

.process-reason[open] .process-reasoning {
  margin-top: 5px;
}

.process-reasoning {
  margin: 0;
  padding-left: 10px;
  border-left: 2px solid var(--outline);
  color: var(--muted);
  font-size: 12.5px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.process-text {
  margin: 0;
  color: var(--ink);
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.process-tool {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--muted);
  font-family: var(--font-mono);
  font-size: 12px;
}

.process-dot {
  width: 6px;
  height: 6px;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--muted);
}

.process-dot.run {
  background: var(--accent);
  animation: process-pulse 1.1s ease-in-out infinite;
}

.process-dot.done {
  background: #1e7d3c;
}

.process-dot.fail {
  background: var(--danger);
}

@keyframes process-pulse {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.3;
  }
}

.branch {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--muted);
  font-size: 12px;
}

/* 错误提示作为消息流的一部分，不再占输入框上方的位置 */
.chat-error {
  padding: 10px 14px;
  border: 1px solid color-mix(in srgb, var(--danger) 32%, transparent);
  border-radius: 10px;
  background: color-mix(in srgb, var(--danger) 8%, transparent);
  color: var(--danger);
  font-size: 13px;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

/* ===== 输入框：始终贴底 ===== */
.composer-wrap {
  width: 100%;
  max-width: 1100px;
  margin: 0 auto 18px;
  padding: 0 28px;
}

/* 托盘常驻：灰底包住输入框，文件夹标签只在托盘上沿露出来 */
/* 托盘：顶部工作区横栏 + 下面白底输入框，横栏铺满整个宽度 */
.composer-frame {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: 6px;
  border: 1px solid var(--outline);
  border-radius: 16px;
  background: var(--page);
}

.workspace-bar {
  display: flex;
  align-items: center;
  gap: 2px;
  height: 28px;
  padding: 0 4px;
  white-space: nowrap;
}

.persona-anchor {
  position: relative;
  margin-left: auto;
}

.persona-menu {
  right: 0;
  left: auto;
  top: calc(100% + 6px);
  bottom: auto;
  width: 190px;
}

.composer {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 12px 9px;
  border: 1px solid var(--outline);
  border-radius: 11px;
  background: var(--surface);
  transition: border-color 150ms ease;
}

.composer:focus-within {
  border-color: var(--accent);
}

.composer-input {
  width: 100%;
  min-height: 44px;
  max-height: 240px;
  padding: 2px 4px;
  border: 0;
  background: transparent;
  color: var(--ink);
  font-size: 15px;
  line-height: 1.7;
  resize: none;
  field-sizing: content;
}

.composer-input:focus {
  outline: none;
}

.composer-input::placeholder {
  color: var(--muted);
}

.composer-bar {
  display: flex;
  align-items: center;
  gap: 2px;
}

.bar-spacer {
  flex: 1;
}

/* 模型选择 */
.model-anchor {
  position: relative;
}

/* 对话设置 */
.settings-anchor {
  position: relative;
  flex-shrink: 0;
}

.chat-settings-btn[aria-expanded='true'] {
  background: var(--tonal);
  color: var(--accent);
}

.settings-menu {
  right: 0;
  bottom: calc(100% + 10px);
  width: 280px;
}

.settings-persona-name {
  text-align: right;
}

.settings-persona-flyout {
  width: 244px;
  max-height: min(360px, calc(100vh - 16px));
  overflow-y: auto;
}

.persona-option {
  display: flex;
  align-items: center;
  height: var(--menu-row-h);
  border-radius: 8px;
  color: var(--ink);
  transition: background-color 120ms ease;
}

.persona-option:hover,
.persona-option:focus-within {
  background: var(--tonal);
}

.persona-option.on {
  background: var(--tonal);
  color: var(--accent);
}

.persona-option-select {
  display: flex;
  align-items: center;
  align-self: stretch;
  gap: 8px;
  flex: 1;
  min-width: 0;
  padding: 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  font-size: 13px;
  text-align: left;
}

.persona-option-select:disabled {
  cursor: default;
}

.persona-detail-btn {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  margin-right: 4px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  opacity: 0;
  pointer-events: none;
  transition: opacity 120ms ease, background-color 120ms ease;
}

.persona-option:hover .persona-detail-btn,
.persona-option:focus-within .persona-detail-btn {
  opacity: 1;
  pointer-events: auto;
}

.persona-detail-btn:hover {
  background: var(--outline);
  color: var(--ink);
}

@media (hover: none) {
  .persona-detail-btn {
    opacity: 1;
    pointer-events: auto;
  }
}

/* 上下文占用圆环 */
.ctx {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  color: var(--muted);
  cursor: help;
  border-radius: 50%;
}

.ctx-ring {
  width: 16px;
  height: 16px;
  /* 让 0% 从 12 点方向开始 */
  transform: rotate(-90deg);
}

.ctx-ring-bg,
.ctx-ring-fill {
  fill: none;
  stroke-width: 2.5;
}

.ctx-ring-bg {
  stroke: var(--outline);
}

.ctx-ring-fill {
  stroke: var(--accent);
  stroke-linecap: round;
  stroke-dasharray: 50.27;
  /* 2πr，r=8 */
  transition: stroke-dashoffset 300ms ease;
}

/* 上下文窗口未知时只留一个淡淡的环 */
.ctx-ring-fill.unknown {
  opacity: 0.3;
}

.ctx::after {
  position: absolute;
  right: -4px;
  bottom: calc(100% + 8px);
  z-index: 40;
  width: max-content;
  max-width: 260px;
  padding: 6px 9px;
  border: 1px solid var(--outline);
  border-radius: 8px;
  background: var(--surface);
  box-shadow: 0 4px 14px rgb(32 38 55 / 12%);
  color: var(--ink);
  content: attr(data-tooltip);
  font-size: 11.5px;
  line-height: 1.5;
  white-space: normal;
  pointer-events: none;
  opacity: 0;
  visibility: hidden;
  transform: translateY(3px);
  transition: opacity 120ms ease, transform 120ms ease, visibility 120ms;
}

.ctx:hover::after,
.ctx:focus-visible::after {
  opacity: 1;
  visibility: visible;
  transform: translateY(0);
}

/* 弹出面板：贴住按钮向上展开，右边缘对齐 */
.model-menu {
  right: 0;
  bottom: calc(100% + 10px);
  width: 300px;
}

/* ===== 权限菜单 ===== */
.perm-anchor {
  position: relative;
}

.perm-menu {
  left: 0;
  bottom: calc(100% + 10px);
  width: 210px;
}

/* ===== 工具配置弹窗（占位） ===== */
.overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: grid;
  place-items: center;
  padding: 20px;
  background: rgb(22 29 50 / 42%);
}

.tools-dialog {
  display: flex;
  flex-direction: column;
  width: min(560px, 100%);
  height: min(560px, 82vh);
  border-radius: 14px;
  background: var(--surface);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
  overflow: hidden;
}

.tools-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--outline);
}

.tools-head h2 {
  font-size: 16px;
  font-weight: 600;
}

.tools-sub {
  margin-top: 3px;
  color: var(--muted);
  font-size: 12px;
}

.tools-body {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding: 16px 20px;
  overflow-y: auto;
}

.tools-empty {
  padding: 32px 0;
  color: var(--muted);
  font-size: 13px;
  text-align: center;
}

.tool-groups {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.tool-group {
  padding: 12px 14px;
  border: 1px solid var(--outline);
  border-radius: 10px;
}

.tool-group-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

/* 展开/折叠按钮：占满左侧，整行可点 */
.tool-expand {
  display: flex;
  align-items: center;
  gap: 7px;
  flex: 1;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--ink);
  text-align: left;
}

.tool-expand:hover .tool-set-name {
  color: var(--accent);
}

.tool-expand .chev {
  width: 13px;
  height: 13px;
  flex-shrink: 0;
  fill: none;
  stroke: var(--muted);
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  transition: transform 150ms ease;
}

.tool-expand .chev.collapsed {
  transform: rotate(-90deg);
}

.tool-set-name {
  font-size: 13.5px;
  font-weight: 600;
}

.tool-count {
  flex-shrink: 0;
  color: var(--muted);
  font-size: 11.5px;
  font-weight: 400;
}

.tool-desc {
  margin-top: 3px;
  color: var(--muted);
  font-size: 11.5px;
  line-height: 1.5;
}

.tool-kits {
  margin-top: 10px;
}

.tool-row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 9px 0;
  border-top: 1px solid var(--outline);
  cursor: pointer;
}

.tool-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.tool-name {
  color: var(--ink);
  font-size: 13px;
}

.tool-text .tool-desc {
  margin-top: 0;
}

.tools-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 20px;
  border-top: 1px solid var(--outline);
}
</style>
