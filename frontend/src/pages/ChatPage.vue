<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { onChatDelta } from '../api'
import { useChatStore } from '../stores/chat'
import { useConfigsStore } from '../stores/configs'

const chat = useChatStore()
const configs = useConfigsStore()
const scroller = ref<HTMLElement | null>(null)
let offDelta: (() => void) | null = null

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
          还没有可用的模型，<RouterLink to="/configs">先去配置</RouterLink>。
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
  </section>
</template>
