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
const providerOptionsOpen = ref(false)
const modelOptionsOpen = ref(false)
const activeConfig = computed(
  () => configs.configs.find((item) => item.ID === chat.session?.configId) ?? configs.enables,
)
const activeModel = computed(() => chat.session?.model || activeConfig.value?.EnabledModel || '')
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
    providerOptionsOpen.value = false
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
    modelOptionsOpen.value = false
  } catch (err) {
    chat.error = String(err)
  }
}

function closeModelMenuOnOutside(event: PointerEvent) {
  if (modelMenuAnchor.value && !modelMenuAnchor.value.contains(event.target as Node)) {
    modelMenuOpen.value = false
    providerOptionsOpen.value = false
    modelOptionsOpen.value = false
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
  onChatDelta((delta) => chat.applyDelta(delta)).then((off) => offs.push(off))
  onChatTool((tool) => chat.applyTool(tool)).then((off) => offs.push(off))
  if (!configs.loaded) configs.load()
  if (!chat.chats.length) chat.loadChats().catch((err) => { chat.error = String(err) })
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', closeModelMenuOnOutside)
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
    <div ref="scroller" class="chat-scroll">
      <div v-if="!chat.messages.length" class="chat-empty">
        <!-- 背景水印:不做任何功能说明 -->
        <p class="watermark">caliniya's agent frame</p>
        <p v-if="!configs.enables || !configs.enabledModel" class="muted hero-meta">
          还没有可用的模型，<RouterLink to="/settings/models">先去配置</RouterLink>。
        </p>
      </div>

      <div
        v-for="(m, i) in chat.messages"
        :key="`${m.nodeId}:${i}`"
        class="msg"
        :class="m.role"
      >
        <span class="msg-role">{{ m.role === 'user' ? '你' : '助手' }}</span>

        <template v-if="m.role === 'assistant'">
          <!-- 思考过程:流式输出中自动展开,结束后折叠 -->
          <details
            v-if="m.reasoning"
            class="reasoning"
            :open="chat.sending && i === chat.messages.length - 1"
          >
            <summary>
              已深度思考<template v-if="chat.sending && i === chat.messages.length - 1">中…</template>
            </summary>
            <div class="reasoning-body">{{ m.reasoning }}</div>
          </details>

          <!-- 工具调用卡片 -->
          <div v-for="(tc, ti) in m.tools ?? []" :key="ti" class="tool-card">
            <span class="tool-icon" :class="tc.status" aria-hidden="true">
              {{ tc.status === 'run' ? '⟳' : '✓' }}
            </span>
            <span class="tool-name mono">{{ tc.name }}</span>
            <span class="tool-status">{{ tc.status === 'run' ? '运行中' : '完成' }}</span>
          </div>
        </template>

        <div class="msg-body">
          {{ m.content }}<span
            v-if="chat.sending && m.role === 'assistant' && i === chat.messages.length - 1"
            class="cursor"
            aria-hidden="true"
          >▍</span>
        </div>
        <div v-if="m.branchCount && m.branchCount > 1" class="branch-controls">
          <button type="button" title="上一个分支" aria-label="上一个分支" @click="selectBranch(m, -1)">‹</button>
          <span>{{ (m.branchIndex ?? 0) + 1 }} / {{ m.branchCount }}</span>
          <button type="button" title="下一个分支" aria-label="下一个分支" @click="selectBranch(m, 1)">›</button>
        </div>
        <button
          v-if="m.role === 'assistant' && m.nodeId && !chat.sending"
          type="button"
          class="regenerate-btn"
          title="重新生成"
          @click="chat.regenerate(m)"
        >
          重新生成
        </button>
      </div>
    </div>

    <div v-if="chat.error" class="notice chat-error">{{ chat.error }}</div>

    <!-- 盒式输入框:输入与全部选项都在框内 -->
    <div class="composer">
      <textarea
        v-model="chat.draft"
        class="composer-input"
        rows="2"
        placeholder="输入消息，Enter 发送，Shift+Enter 换行"
        @keydown.enter.exact="onComposerEnter"
      ></textarea>

      <div class="composer-meta">
        <label class="composer-chip">
          权限
          <select v-model="chat.permission">
            <option>只读</option>
            <option>标准</option>
            <option>完全访问</option>
          </select>
        </label>

        <div class="composer-right">
          <div ref="modelMenuAnchor" class="model-menu-anchor">
            <button
              type="button"
              class="badge model-menu-trigger"
              :class="{ 'badge-active': activeModel }"
              :aria-expanded="modelMenuOpen"
              aria-haspopup="true"
              @click="modelMenuOpen = !modelMenuOpen; providerOptionsOpen = false; modelOptionsOpen = false"
            >
              {{ activeModel || '未配置模型' }}
              <span class="menu-chevron" aria-hidden="true">⌃</span>
            </button>
            <div v-if="modelMenuOpen" class="model-menu" @keydown.esc.stop="modelMenuOpen = false">
              <div class="model-menu-section">
                <span class="model-menu-label">模型提供商</span>
                <div class="model-menu-item">
                  <button
                    type="button"
                    class="model-menu-row"
                    :aria-expanded="providerOptionsOpen"
                    @click="providerOptionsOpen = !providerOptionsOpen; modelOptionsOpen = false"
                  >
                    <span>{{ activeConfig?.Name || activeConfig?.ServerAddress || '未选择提供商' }}</span>
                    <span class="menu-chevron" aria-hidden="true">{{ providerOptionsOpen ? '‹' : '›' }}</span>
                  </button>
                  <div v-if="providerOptionsOpen" class="model-menu-options model-menu-flyout">
                    <span class="model-menu-flyout-title">选择提供商</span>
                    <button
                      v-for="(config, index) in configs.configs"
                      :key="config.ID"
                      type="button"
                      class="model-menu-option"
                      :class="{ selected: config.ID === activeConfig?.ID }"
                      @click="selectConfig(config.ID)"
                    >
                      {{ config.Name || config.ServerAddress || `配置 ${index + 1}` }}
                    </button>
                    <span v-if="!configs.configs.length" class="model-menu-empty">暂无已配置的提供商</span>
                  </div>
                </div>
              </div>

              <div class="model-menu-section">
                <span class="model-menu-label">模型</span>
                <div class="model-menu-item">
                  <button
                    type="button"
                    class="model-menu-row"
                    :aria-expanded="modelOptionsOpen"
                    :disabled="!activeConfig?.Model?.length"
                    @click="modelOptionsOpen = !modelOptionsOpen; providerOptionsOpen = false"
                  >
                    <span>{{ activeConfig?.Model.find((item) => item.id === activeModel)?.name || activeModel || '未选择模型' }}</span>
                    <span class="menu-chevron" aria-hidden="true">{{ modelOptionsOpen ? '‹' : '›' }}</span>
                  </button>
                  <div v-if="modelOptionsOpen" class="model-menu-options model-menu-flyout">
                    <span class="model-menu-flyout-title">选择模型</span>
                    <button
                      v-for="model in activeConfig?.Model ?? []"
                      :key="model.id"
                      type="button"
                      class="model-menu-option"
                      :class="{ selected: model.id === activeModel }"
                      @click="selectModel(model.id)"
                    >
                      {{ model.name || model.id }}
                    </button>
                  </div>
                </div>
              </div>

              <RouterLink class="model-menu-settings" to="/settings/models" @click="modelMenuOpen = false">
                打开模型设置
                <span aria-hidden="true">↗</span>
              </RouterLink>
            </div>
          </div>
          <label class="composer-chip">
            思考
            <select v-model="chat.effort">
              <option value="none">关闭</option>
              <option value="low">低</option>
              <option value="high">中</option>
              <option value="max">高</option>
            </select>
          </label>
          <span
            class="ctx"
            :data-tooltip="chat.ctxTooltip"
            :aria-label="chat.ctxTooltip"
            tabindex="0"
          >
            <svg class="ctx-ring" viewBox="0 0 20 20" aria-hidden="true">
              <circle class="ctx-ring-bg" cx="10" cy="10" r="8" />
              <circle
                class="ctx-ring-fill"
                cx="10"
                cy="10"
                r="8"
                :class="{ 'ctx-ring-unknown': chat.ctxPercent === null }"
                :style="{ strokeDashoffset: 50.27 * (1 - (chat.ctxPercent ?? 0) / 100) }"
              />
            </svg>
          </span>

          <button
            type="button"
            class="send-btn"
            :disabled="chat.sending || !chat.draft.trim()"
            aria-label="发送"
            title="发送"
            @click="chat.send()"
          >
            <!-- 向上箭头图标(Feather, MIT):小尺寸下比纸飞机更清晰 -->
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <line x1="12" y1="19" x2="12" y2="5" />
              <polyline points="5 12 12 5 19 12" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
