import { defineStore } from 'pinia'
import { SendMessage, type ChatTurn } from '../api'
import { useConfigsStore } from './configs'

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
}

// 对话状态：消息记录仅保存在内存中，应用关闭即清空。
export const useChatStore = defineStore('chat', {
  state: () => ({
    messages: [] as ChatMessage[],
    draft: '',
    sending: false,
    error: '',
  }),
  actions: {
    // 由 ChatPage 订阅的 "chat:delta" 事件驱动，把增量追加到最后的助手消息。
    applyDelta(delta: string) {
      const last = this.messages[this.messages.length - 1]
      if (last && last.role === 'assistant') last.content += delta
    },
    async send() {
      const text = this.draft.trim()
      if (!text || this.sending) return

      const configs = useConfigsStore()
      if (!configs.enables || !configs.enabledModel) {
        this.error = '请先在「模型配置」中启用提供商并选定模型。'
        return
      }

      this.error = ''
      this.draft = ''
      this.messages.push({ role: 'user', content: text })
      this.messages.push({ role: 'assistant', content: '' })
      this.sending = true

      // 最后一条是占位的空助手消息，发历史时不带它。
      const history: ChatTurn[] = this.messages
        .slice(0, -1)
        .map((m) => ({ role: m.role, content: m.content }))

      try {
        await SendMessage(history)
      } catch (err) {
        this.error = String(err)
        const last = this.messages[this.messages.length - 1]
        if (last?.role === 'assistant' && !last.content) this.messages.pop()
      } finally {
        this.sending = false
      }
    },
  },
})
