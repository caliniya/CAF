<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { onChatDelta, onChatTool } from '../api'
import { useChatStore } from '../stores/chat'
import { useConfigsStore } from '../stores/configs'

const chat = useChatStore()
const configs = useConfigsStore()
const scroller = ref<HTMLElement | null>(null)
const modelMenuAnchor = ref<HTMLElement | null>(null)
const modelMenuOpen = ref(false)

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
    return index < 0 ? 0 : index
  },
  set: (index: number) => {
    const step = effortSteps[Math.round(index)]
    if (step) chat.effort = step.value
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

function closeModelMenuOnOutside(event: PointerEvent) {
  if (modelMenuAnchor.value && !modelMenuAnchor.value.contains(event.target as Node)) {
    modelMenuOpen.value = false
    openFlyout.value = ''
  }
}

// Enter 发送；输入法组合输入（拼音候选）期间不能当作发送。
function onComposerEnter(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  chat.send()
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
  document.addEventListener('pointerdown', closeModelMenuOnOutside)
  document.addEventListener('keydown', onGlobalKeydown)
  onChatDelta((delta) => chat.applyDelta(delta)).then((off) => offs.push(off))
  onChatTool((tool) => chat.applyTool(tool)).then((off) => offs.push(off))
  if (!configs.loaded) configs.load()
  if (!chat.chats.length) chat.loadChats().catch((err) => { chat.error = String(err) })
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', closeModelMenuOnOutside)
  document.removeEventListener('keydown', onGlobalKeydown)
  offs.forEach((off) => off())
})

watch(
  () => [chat.session?.id, chat.session?.configId, chat.session?.model],
  () => chat.loadContextWindow(),
  { immediate: true },
)

// 消息条数、正文/思考/工具卡变化(流式增量)时滚到底部。
watch(
  () => {
    const last = chat.messages[chat.messages.length - 1]
    return `${chat.messages.length}:${last?.content.length ?? 0}:${last?.reasoning?.length ?? 0}:${last?.tools?.length ?? 0}`
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
    <div ref="scroller" class="chat-body">
      <div
        v-for="(m, i) in chat.messages"
        :key="`${m.nodeId}:${i}`"
        class="msg"
        :class="m.role"
      >
        <details
          v-if="m.role === 'assistant' && m.reasoning"
          class="reasoning"
          :open="chat.sending && i === chat.messages.length - 1"
        >
          <summary>已深度思考</summary>
          <div class="reasoning-body">{{ m.reasoning }}</div>
        </details>

        <div v-for="(tc, ti) in m.tools ?? []" :key="ti" class="tool-card">
          <span class="tool-icon" :class="tc.status" aria-hidden="true">
            {{ tc.status === 'run' ? '⟳' : '✓' }}
          </span>
          <span class="tool-name">{{ tc.name }}</span>
        </div>

        <div class="msg-body">
          {{ m.content }}<span
            v-if="chat.sending && m.role === 'assistant' && i === chat.messages.length - 1"
            class="cursor"
            aria-hidden="true"
          >▍</span>
        </div>

        <div v-if="m.branchCount && m.branchCount > 1" class="branch">
          <button type="button" aria-label="上一个分支" @click="selectBranch(m, -1)">‹</button>
          <span>{{ (m.branchIndex ?? 0) + 1 }} / {{ m.branchCount }}</span>
          <button type="button" aria-label="下一个分支" @click="selectBranch(m, 1)">›</button>
        </div>

        <button
          v-if="m.role === 'assistant' && m.nodeId && !chat.sending"
          type="button"
          class="regen"
          @click="chat.regenerate(m)"
        >
          重新生成
        </button>
      </div>
    </div>

    <p v-if="chat.error" class="chat-error">{{ chat.error }}</p>

    <div class="composer-wrap">
      <div class="composer">
        <div class="composer-context">
          <button type="button" class="ctx-pill">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
            </svg>
            <span>CAF</span>
          </button>
          <button type="button" class="ctx-add" title="添加上下文" aria-label="添加上下文">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
          </button>
        </div>

        <textarea
          v-model="chat.draft"
          class="composer-input"
          rows="1"
          placeholder="向 CAF 提问"
          @keydown.enter.exact="onComposerEnter"
        ></textarea>

        <div class="composer-bar">
          <button type="button" class="bar-icon" title="附件" aria-label="附件">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
          </button>

          <label class="perm" :class="{ elevated: chat.permission === '完全访问' }">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M12 3l7 3v6c0 4.5-3 8-7 9-4-1-7-4.5-7-9V6z" />
            </svg>
            <select v-model="chat.permission" aria-label="权限">
              <option>只读</option>
              <option>标准</option>
              <option>完全访问</option>
            </select>
            <svg class="chev" viewBox="0 0 24 24" aria-hidden="true">
              <polyline points="6 9 12 15 18 9" />
            </svg>
          </label>

          <span class="bar-spacer"></span>

          <!-- 上下文占用：仅在有对话内容时出现，悬停看完整数值 -->
          <span
            v-if="chat.messages.length"
            class="ctx"
            :data-tooltip="chat.ctxTooltip"
            :aria-label="chat.ctxTooltip"
            tabindex="0"
          >
            <svg class="ctx-ring" viewBox="0 0 20 20" aria-hidden="true">
              <circle class="ctx-ring-bg" cx="10" cy="10" r="8" />
              <circle
                class="ctx-ring-fill"
                :class="{ unknown: chat.ctxPercent === null }"
                cx="10"
                cy="10"
                r="8"
                :style="{ strokeDashoffset: 50.27 * (1 - (chat.ctxPercent ?? 0) / 100) }"
              />
            </svg>
          </span>

          <div ref="modelMenuAnchor" class="model-anchor">
            <button
              type="button"
              class="model-btn"
              :aria-expanded="modelMenuOpen"
              aria-haspopup="menu"
              @click="modelMenuOpen = !modelMenuOpen"
            >
              <span class="model-btn-label">{{ activeModel || '选择模型' }}</span>
              <svg class="chev" viewBox="0 0 24 24" aria-hidden="true">
                <polyline points="6 9 12 15 18 9" />
              </svg>
            </button>

            <div v-if="modelMenuOpen" class="menu model-menu" role="menu">
              <!-- 提供商 -->
              <div class="menu-slot">
                <button
                  type="button"
                  class="menu-row"
                  :aria-expanded="openFlyout === 'provider'"
                  @click="toggleFlyout('provider', $event)"
                >
                  <svg class="row-icon" viewBox="0 0 24 24" aria-hidden="true">
                    <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z" />
                    <path d="M12 12l8-4.5M12 12v9M12 12L4 7.5" />
                  </svg>
                  <span class="menu-row-name">
                    {{ activeConfig?.Name || activeConfig?.ServerAddress || '未选择提供商' }}
                  </span>
                  <span class="tag">API</span>
                  <svg class="chev" viewBox="0 0 24 24" aria-hidden="true">
                    <polyline points="9 6 15 12 9 18" />
                  </svg>
                </button>

                <div
                  v-if="openFlyout === 'provider'"
                  class="flyout"
                  :class="{ 'flyout--flip': flyoutFlip }"
                  role="menu"
                >
                  <button
                    v-for="config in configs.configs"
                    :key="config.ID"
                    type="button"
                    class="menu-row"
                    :class="{ on: config.ID === activeConfig?.ID }"
                    @click="selectConfig(config.ID)"
                  >
                    <svg class="row-icon" viewBox="0 0 24 24" aria-hidden="true">
                      <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9z" />
                      <path d="M12 12l8-4.5M12 12v9M12 12L4 7.5" />
                    </svg>
                    <span class="menu-row-name">{{ config.Name || config.ServerAddress }}</span>
                    <span class="tag">API</span>
                    <span class="menu-spacer"></span>
                    <svg
                      v-if="config.ID === activeConfig?.ID"
                      class="check"
                      viewBox="0 0 24 24"
                      aria-hidden="true"
                    >
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
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
                <button
                  type="button"
                  class="menu-row"
                  :aria-expanded="openFlyout === 'model'"
                  @click="toggleFlyout('model', $event)"
                >
                  <span class="menu-row-label">模型</span>
                  <span class="menu-spacer"></span>
                  <span class="menu-value">{{ activeModelName }}</span>
                  <svg class="chev" viewBox="0 0 24 24" aria-hidden="true">
                    <polyline points="9 6 15 12 9 18" />
                  </svg>
                </button>

                <div
                  v-if="openFlyout === 'model'"
                  class="flyout"
                  :class="{ 'flyout--flip': flyoutFlip }"
                  role="menu"
                >
                  <button
                    v-for="model in activeConfig?.Model ?? []"
                    :key="model.id"
                    type="button"
                    class="menu-item"
                    :class="{ on: model.id === activeModel }"
                    @click="selectModel(model.id)"
                  >
                    <span class="menu-item-name">{{ model.name || model.id }}</span>
                    <span v-if="model.input_modalities?.includes('image')" class="tag">视觉</span>
                    <svg
                      v-if="model.id === activeModel"
                      class="check"
                      viewBox="0 0 24 24"
                      aria-hidden="true"
                    >
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
                  </button>
                  <span v-if="!activeConfig?.Model?.length" class="menu-empty">暂无模型</span>
                </div>
              </div>

              <!-- 思考强度：点开二级菜单选档位 -->
              <div class="menu-slot">
                <button
                  type="button"
                  class="menu-row"
                  :aria-expanded="openFlyout === 'effort'"
                  @click="toggleFlyout('effort', $event)"
                >
                  <span class="menu-row-label">思考强度</span>
                  <span class="menu-spacer"></span>
                  <span class="menu-value">{{ effortLabel }}</span>
                  <kbd class="menu-kbd">Ctrl+T</kbd>
                  <svg class="chev" viewBox="0 0 24 24" aria-hidden="true">
                    <polyline points="9 6 15 12 9 18" />
                  </svg>
                </button>

                <div
                  v-if="openFlyout === 'effort'"
                  class="flyout flyout--narrow"
                  :class="{ 'flyout--flip': flyoutFlip }"
                  role="menu"
                >
                  <button
                    v-for="(step, index) in effortSteps"
                    :key="step.value"
                    type="button"
                    class="menu-row"
                    :class="{ on: index === effortIndex }"
                    @click="pickEffort(index)"
                  >
                    <span class="menu-row-name">{{ step.label }}</span>
                    <svg v-if="index === effortIndex" class="check" viewBox="0 0 24 24" aria-hidden="true">
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
                  </button>
                </div>
              </div>

              <div class="menu-sep"></div>

              <RouterLink class="menu-row" to="/settings/models" @click="modelMenuOpen = false">
                模型配置
              </RouterLink>
            </div>
          </div>

          <button
            type="button"
            class="send"
            :disabled="!chat.draft.trim() || chat.sending"
            aria-label="发送"
            @click="chat.send()"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <line x1="12" y1="19" x2="12" y2="6" />
              <polyline points="6 12 12 6 18 12" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.chat {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

/* 消息区：唯一滚动容器 */
.chat-body {
  display: flex;
  flex-direction: column;
  gap: 18px;
  flex: 1;
  min-height: 0;
  width: 100%;
  max-width: 868px;
  margin: 0 auto;
  padding: 24px 24px 8px;
  overflow-y: auto;
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
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.msg.user .msg-body {
  max-width: 78%;
  padding: 9px 14px;
  border-radius: 12px;
  background: var(--tonal);
  color: var(--ink);
}

.cursor {
  animation: blink 1s steps(2, start) infinite;
}

@keyframes blink {
  to {
    visibility: hidden;
  }
}

.reasoning {
  width: 100%;
  border-left: 2px solid var(--outline);
  border-radius: 4px;
  background: color-mix(in srgb, var(--tonal) 45%, transparent);
}

.reasoning summary {
  padding: 5px 10px;
  color: var(--muted);
  font-size: 12.5px;
  cursor: pointer;
  user-select: none;
}

.reasoning-body {
  padding: 0 12px 9px;
  color: var(--muted);
  font-size: 12.5px;
  line-height: 1.6;
  white-space: pre-wrap;
}

.tool-card {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 5px 10px;
  border: 1px solid var(--outline);
  border-radius: 8px;
  font-size: 12.5px;
}

.tool-icon {
  display: inline-flex;
  width: 16px;
  height: 16px;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  font-size: 11px;
}

.tool-icon.run {
  background: var(--tonal);
  color: var(--accent);
}

.tool-icon.done {
  background: #e3f5e9;
  color: #1e7d3c;
}

.tool-name {
  font-family: var(--font-mono);
}

.branch {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--muted);
  font-size: 12px;
}

.chat-error {
  max-width: 868px;
  width: 100%;
  margin: 0 auto;
  padding: 8px 24px;
  color: var(--danger);
  font-size: 13px;
}

/* ===== 输入框：始终贴底 ===== */
.composer-wrap {
  width: 100%;
  max-width: 868px;
  margin: 0 auto 16px;
  padding: 0 24px;
}

.composer {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 9px 10px 7px;
  border: 1px solid transparent;
  border-radius: 14px;
  background: var(--page);
  transition: border-color 150ms ease, background-color 150ms ease;
}

.composer:focus-within {
  border-color: var(--outline);
  background: var(--surface);
}

.composer-context {
  display: flex;
  align-items: center;
  gap: 4px;
  padding-left: 2px;
}

.composer-input {
  width: 100%;
  min-height: 26px;
  max-height: 200px;
  padding: 0 4px;
  border: 0;
  background: transparent;
  color: var(--ink);
  font-size: 14px;
  line-height: 1.6;
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

/* 上下文占用圆环 */
.ctx {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  color: var(--muted);
  cursor: help;
  border-radius: 50%;
}

.ctx-ring {
  width: 15px;
  height: 15px;
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
  stroke-dasharray: 50.27; /* 2πr，r=8 */
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
</style>
