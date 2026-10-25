<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { onChatDelta, onChatTool } from '../api'
import { useChatStore } from '../stores/chat'
import { useConfigsStore } from '../stores/configs'

const chat = useChatStore()
const configs = useConfigsStore()
const scroller = ref<HTMLElement | null>(null)
let offs: (() => void)[] = []

onMounted(() => {
  onChatDelta((delta) => chat.applyDelta(delta)).then((off) => offs.push(off))
  onChatTool((tool) => chat.applyTool(tool)).then((off) => offs.push(off))
  if (!configs.loaded) configs.load()
})
onUnmounted(() => offs.forEach((off) => off()))

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
        <p v-if="configs.enables && configs.enabledModel" class="muted hero-meta">
          当前模型：<span class="badge badge-active">{{ configs.enabledModel }}</span>
        </p>
        <p v-else class="muted hero-meta">
          还没有可用的模型，<RouterLink to="/settings/models">先去配置</RouterLink>。
        </p>
      </div>

      <div
        v-for="(m, i) in chat.messages"
        :key="i"
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
      </div>
    </div>

    <div v-if="chat.error" class="notice chat-error">{{ chat.error }}</div>

    <!-- 新建工作区:位于输入框外、左上位置 -->
    <div class="composer-top">
      <button type="button" class="new-workspace" @click="chat.addWorkspace()">
        ＋ 新建工作区
      </button>
    </div>

    <!-- 盒式输入框:输入与全部选项都在框内 -->
    <div class="composer">
      <textarea
        v-model="chat.draft"
        class="composer-input"
        rows="2"
        placeholder="输入消息，Enter 发送，Shift+Enter 换行"
        @keydown.enter.exact.prevent="chat.send()"
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
          <span class="badge" :class="{ 'badge-active': configs.enabledModel }">
            {{ configs.enabledModel || '未配置模型' }}
          </span>
          <label class="composer-chip">
            思考
            <select v-model="chat.effort">
              <option>关闭</option>
              <option>低</option>
              <option>中</option>
              <option>高</option>
            </select>
          </label>
          <span class="ctx" title="上下文占用（演示用假数据）">
            <span class="ctx-bar">
              <span class="ctx-fill" :style="{ width: chat.ctxPercent + '%' }"></span>
            </span>
            {{ chat.ctxPercent }}%
          </span>

          <button
            type="button"
            class="send-btn"
            :disabled="chat.sending || !chat.draft.trim()"
            aria-label="发送"
            title="发送"
            @click="chat.send()"
          >
            <!-- 纸飞机图标(Feather, MIT) -->
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <line x1="22" y1="2" x2="11" y2="13" />
              <polygon points="22 2 15 22 11 13 2 9 22 2" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
