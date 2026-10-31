import { defineStore } from 'pinia'
import {
  CreateChat,
  DeleteChat,
  GetChatModelContextWindow,
  GetChat,
  ListChats,
  RegenerateChatMessage,
  SendChatMessage,
  SetChatBranch,
  UpdateChatModel,
  type ChatTask as StoredChatTask,
  type ChatSummary,
  type ChatDeltaEvent,
  type ChatToolEvent,
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
  nodeId?: string
  parentId?: string
  branchIndex?: number
  branchCount?: number
}

export const useChatStore = defineStore('chat', {
  state: () => ({
    chats: [] as ChatSummary[],
    session: null as StoredChatTask | null,
    messages: [] as ChatMessage[],
    draft: '',
    sending: false,
    error: '',
    usage: null as TokenUsage | null,
    contextWindow: null as number | null,
    // 输入框状态行的界面设置(演示阶段仅前端本地)。
    permission: '标准',
    effort: 'none' as 'none' | 'low' | 'high' | 'max',
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
    reset() {
      this.session = null
      this.messages = []
      this.error = ''
      this.sending = false
      this.usage = null
    },
    async loadContextWindow() {
      const requestId = ++contextWindowRequestId
      this.contextWindow = null
      if (!this.session) return
      try {
        const contextWindow = await GetChatModelContextWindow(this.session.id)
        if (requestId === contextWindowRequestId) this.contextWindow = contextWindow
      } catch {
        // Compatible providers may not expose DeepSeek's model metadata endpoint.
      }
    },
    applyDelta(delta: ChatDeltaEvent) {
      if (delta.chatId && delta.chatId !== this.session?.id) return
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
      if (tool.chatId && tool.chatId !== this.session?.id) return
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

      if (!this.session) {
        try {
          await this.createChat()
        } catch (err) {
          this.error = String(err)
          return
        }
      }
      if (!this.session?.configId || !this.session.model) {
        this.error = '请先为此对话选择提供商和模型。'
        return
      }

      this.error = ''
      this.usage = null
      this.draft = ''
      this.messages.push({ role: 'user', content: text })
      this.messages.push({ role: 'assistant', content: '' })
      this.sending = true

      try {
        await SendChatMessage(this.session.id, text, this.effort)
        await this.reloadSession()
      } catch (err) {
        this.error = String(err)
        await this.reloadSession().catch(() => undefined)
      } finally {
        this.sending = false
      }
    },
    rebuildMessages() {
      const session = this.session
      this.messages = []
      const root = session?.tree?.root
      if (!root || root.choice < 0 || root.choice >= root.next.length) return
      let node = root.next[root.choice]
      let parentId = ''
      while (node) {
        const siblings = parentId ? this.findNode(parentId)?.next ?? [] : root.next
        const branchIndex = parentId ? this.findNode(parentId)?.choice ?? 0 : root.choice
        for (const message of node.child ?? []) {
          const role = message.Role === 'user' ? 'user' : 'assistant'
          this.messages.push({
            role,
            content: message.Content ?? '',
            reasoning: message.Reasoning,
            tools: (message.ToolCalls ?? []).map((tool) => ({ name: tool.Name, status: 'done' as const })),
            nodeId: node.id,
            parentId,
            branchIndex,
            branchCount: siblings.length,
          })
        }
        parentId = node.id
        if (!node.next?.length || node.choice < 0 || node.choice >= node.next.length) break
        node = node.next[node.choice]
      }
    },
    findNode(id: string) {
      const root = this.session?.tree?.root
      if (!root) return undefined
      const pending = [...root.next]
      const seen = new Set<object>()
      while (pending.length) {
        const node = pending.pop()!
        if (seen.has(node)) continue
        seen.add(node)
        if (node.id === id) return node
        pending.push(...(node.next ?? []))
      }
      return undefined
    },
    async loadChats() {
      this.chats = await ListChats()
    },
    async createChat() {
      const configs = useConfigsStore()
      const session = await CreateChat(configs.enables?.ID ?? '', configs.enabledModel, '')
      this.session = session
      this.rebuildMessages()
      await this.loadChats()
      return session
    },
    async openChat(id: string) {
      this.error = ''
      this.session = await GetChat(id)
      this.rebuildMessages()
    },
    async reloadSession() {
      if (!this.session) return
      this.session = await GetChat(this.session.id)
      this.rebuildMessages()
      await this.loadChats()
    },
    async deleteChat(id: string) {
      await DeleteChat(id)
      this.chats = this.chats.filter((chat) => chat.id !== id)
      if (this.session?.id === id) this.reset()
    },
    async selectBranch(parentId: string, choice: number) {
      if (!this.session) return
      this.session = await SetChatBranch(this.session.id, parentId, choice)
      this.rebuildMessages()
    },
    async updateModel(configId: string, model: string) {
      if (!this.session) return
      this.session = await UpdateChatModel(this.session.id, configId, model)
      await this.loadContextWindow()
      await this.loadChats()
    },
    async regenerate(message: ChatMessage) {
      if (!this.session || !message.nodeId || message.role !== 'assistant' || this.sending) return
      const index = this.messages.indexOf(message)
      this.messages = this.messages.slice(0, index + 1)
      const pending = this.messages[index]
      pending.content = ''
      pending.reasoning = ''
      pending.tools = []
      this.error = ''
      this.sending = true
      try {
        await RegenerateChatMessage(this.session.id, message.nodeId, this.effort)
        await this.reloadSession()
      } catch (err) {
        this.error = String(err)
        await this.reloadSession().catch(() => undefined)
      } finally {
        this.sending = false
      }
    },
  },
})
