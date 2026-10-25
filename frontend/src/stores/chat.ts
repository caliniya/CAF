import { defineStore } from 'pinia'
import { SendMessage, type ChatDeltaEvent, type ChatToolEvent, type ChatTurn } from '../api'
import { useConfigsStore } from './configs'

export interface ToolCard {
  name: string
  status: 'run' | 'done'
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  reasoning?: string
  tools?: ToolCard[]
}

export interface WorkspaceEntry {
  name: string
  conversations: { id: number; title: string }[]
}

// 对话状态：消息记录仅保存在内存中，应用关闭即清空。
export const useChatStore = defineStore('chat', {
  state: () => ({
    messages: [] as ChatMessage[],
    draft: '',
    sending: false,
    error: '',
    // 输入框状态行的界面设置(演示阶段仅前端本地)。
    permission: '标准',
    effort: '关闭',
    // 工作区列表(演示阶段仅前端本地,新建对话不清除)。
    workspaces: [] as WorkspaceEntry[],
  }),
  getters: {
    // 假的上下文占用指示器:按消息总字数粗略增长,仅用于演示。
    ctxPercent(state) {
      const total = state.messages.reduce(
        (n, m) => n + m.content.length + (m.reasoning?.length ?? 0),
        0,
      )
      return Math.min(95, Math.round(total / 30) + 2)
    },
  },
  actions: {
    addWorkspace() {
      const n = this.workspaces.length + 1
      this.workspaces.push({ name: `工作区 ${n}`, conversations: [] })
    },
    // 新建对话：清空当前会话（演示用途，消息本来就只存在内存里）。
    reset() {
      this.messages = []
      this.error = ''
      this.sending = false
    },
    // 由 ChatPage 订阅的 "chat:delta" 事件驱动：内容与思考增量分别追加
    // 到最后的助手消息上（当前正在流式输出的那条）。
    applyDelta(delta: ChatDeltaEvent) {
      const last = this.messages[this.messages.length - 1]
      if (!last || last.role !== 'assistant') return
      if (delta.reasoning) {
        last.reasoning = (last.reasoning ?? '') + delta.reasoning
      }
      if (delta.content) {
        last.content += delta.content
      }
    },
    // 由 "chat:tool" 事件驱动：往最后的助手消息上添加/完结工具调用卡片。
    applyTool(tool: ChatToolEvent) {
      const last = this.messages[this.messages.length - 1]
      if (!last || last.role !== 'assistant') return
      last.tools = last.tools ?? []
      if (tool.event === 'start') {
        last.tools.push({ name: tool.name, status: 'run' })
        return
      }
      // done:完结最近一张同名且还在运行的卡片。
      for (let i = last.tools.length - 1; i >= 0; i--) {
        if (last.tools[i].name === tool.name && last.tools[i].status === 'run') {
          last.tools[i].status = 'done'
          return
        }
      }
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
        if (last?.role === 'assistant' && !last.content && !last.reasoning && !last.tools?.length) {
          this.messages.pop()
        }
      } finally {
        this.sending = false
      }
    },
  },
})
