import { defineStore } from 'pinia'
import {
  GetModelContextWindow,
  SendMessage,
  type ChatDeltaEvent,
  type ChatToolEvent,
  type ChatTurn,
  type TokenUsage,
} from '../api'
import { useConfigsStore } from './configs'

let contextWindowRequestId = 0

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
    usage: null as TokenUsage | null,
    contextWindow: null as number | null,
    // 输入框状态行的界面设置(演示阶段仅前端本地)。
    permission: '标准',
    effort: 'none' as 'none' | 'low' | 'high' | 'max',
    // 工作区列表(演示阶段仅前端本地,新建对话不清除)。
    workspaces: [] as WorkspaceEntry[],
  }),
  getters: {
    ctxPercent(state): number | null {
      if (!state.usage || !state.contextWindow) return null
      return Math.min(100, (state.usage.total_tokens / state.contextWindow) * 100)
    },
    ctxTooltip(state): string {
      if (!state.usage) return '尚无服务端 token 用量数据'
      const used = state.usage.total_tokens.toLocaleString()
      if (!state.contextWindow) return `本次请求 ${used} tokens · 模型上下文窗口未知`
      const percent = Math.min(100, (state.usage.total_tokens / state.contextWindow) * 100)
      return `上下文约 ${percent.toFixed(1)}% · ${used} / ${state.contextWindow.toLocaleString()} tokens`
    },
  },
  actions: {
    addWorkspace() {
      const n = this.workspaces.length + 1
      this.workspaces.push({ name: `工作区 ${n}`, conversations: [] })
    },
    reset() {
      this.messages = []
      this.error = ''
      this.sending = false
      this.usage = null
    },
    async loadContextWindow() {
      const requestId = ++contextWindowRequestId
      this.contextWindow = null
      try {
        const contextWindow = await GetModelContextWindow()
        if (requestId === contextWindowRequestId) this.contextWindow = contextWindow
      } catch {
        // Compatible providers may not expose DeepSeek's model metadata endpoint.
      }
    },
    applyDelta(delta: ChatDeltaEvent) {
      if (delta.usage) this.usage = delta.usage
      const last = this.messages[this.messages.length - 1]
      if (!last || last.role !== 'assistant') return
      if (delta.reasoning) {
        last.reasoning = (last.reasoning ?? '') + delta.reasoning
      }
      if (delta.content) {
        last.content += delta.content
      }
    },
    applyTool(tool: ChatToolEvent) {
      const last = this.messages[this.messages.length - 1]
      if (!last || last.role !== 'assistant') return
      last.tools = last.tools ?? []
      if (tool.event === 'start') {
        last.tools.push({ name: tool.name, status: 'run' })
        return
      }
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
      this.usage = null
      this.draft = ''
      this.messages.push({ role: 'user', content: text })
      this.messages.push({ role: 'assistant', content: '' })
      this.sending = true

      // 最后一条是占位的空助手消息，发历史时不带它。
      const history: ChatTurn[] = this.messages
        .slice(0, -1)
        .map((m) => ({ role: m.role, content: m.content }))

      try {
        await SendMessage(history, this.effort)
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
