<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { onChatDelta } from '../api'
import { useChatStore } from '../stores/chat'
import { useConfigsStore } from '../stores/configs'

const chat = useChatStore()
const configs = useConfigsStore()
const scroller = ref<HTMLElement | null>(null)
let offDelta: (() => void) | null = null

// 演示用的界面状态：权限与思考强度暂不参与实际请求。
const permission = ref('标准')
const effort = ref('关闭')

// 假的上下文占用指示器：按消息总字数粗略增长，仅用于演示。
const ctxPercent = computed(() => {
  const total = chat.messages.reduce((n, m) => n + m.content.length, 0)
  return Math.min(95, Math.round(total / 300) + 2)
})

onMounted(() => {
  onChatDelta((delta) => chat.applyDelta(delta)).then((off) => {
    offDelta = off
  })
  if (!configs.loaded) configs.load()
})
onUnmounted(() => offDelta?.())

// 消息条数或最后一条内容长度变化（流式增量）时滚到底部。
watch(
  () => {
    const last = chat.messages[chat.messages.length - 1]
    return `${chat.messages.length}:${last?.content.length ?? 0}`
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
        <h3>开始对话</h3>
        <p v-if="configs.enables && configs.enabledModel" class="muted">
          当前模型：<span class="badge badge-active">{{ configs.enabledModel }}</span>
        </p>
        <p v-else class="muted">
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

    <div class="composer">
      <div class="composer-row">
        <textarea
          v-model="chat.draft"
          rows="1"
          placeholder="输入消息，Enter 发送，Shift+Enter 换行"
          @keydown.enter.exact.prevent="chat.send()"
        ></textarea>
        <button
          type="button"
          :disabled="chat.sending || !chat.draft.trim()"
          @click="chat.send()"
        >
          {{ chat.sending ? '回复中…' : '发送' }}
        </button>
      </div>

      <div class="composer-meta">
        <label class="composer-chip">
          权限
          <select v-model="permission">
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
            <select v-model="effort">
              <option>关闭</option>
              <option>低</option>
              <option>中</option>
              <option>高</option>
            </select>
          </label>
          <span
            class="ctx"
            title="上下文占用（演示用假数据）"
          >
            <span class="ctx-bar">
              <span class="ctx-fill" :style="{ width: ctxPercent + '%' }"></span>
            </span>
            {{ ctxPercent }}%
          </span>
        </div>
      </div>
    </div>
  </section>
</template>
