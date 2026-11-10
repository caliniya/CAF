import { defineStore } from 'pinia'
import {
  CreateChat,
  CreateWorkspace,
  DeleteChat,
  DeleteWorkspace,
  DeleteWorkspaceAndChats,
  EditChatMessage,
  GetChatModelContextWindow,
  GetChat,
  ListChats,
  ListPersonas,
  ListWorkspaces,
  RenameChat,
  RenameWorkspace,
  RegenerateChatMessage,
  SendChatMessage,
  SetChatBranch,
  SetChatPersona,
  SetChatReasoningEffort,
  SetChatToolConfig,
  SetChatWorkspace,
  UpdateChatModel,
  type ChatTask as StoredChatTask,
  type ChatSummary,
  type ChatToolConfig,
  type ChatDeltaEvent,
  type ChatToolEvent,
  type Persona,
  type TokenUsage,
  type Workspace,
  type PermissionLevel,
} from '../api'
import { useConfigsStore } from './configs'

let contextWindowRequestId = 0
let effortUpdates: Promise<void> = Promise.resolve()
let toolConfigUpdates: Promise<void> = Promise.resolve()

// 没有历史 token 记录时粗略估算：ASCII 约四个字符一 token，其余字符约一 token。
function estimateTextTokens(text: string): number {
  let ascii = 0
  let nonAscii = 0
  for (const char of text) {
    if (char.codePointAt(0)! <= 0x7f) ascii++
    else nonAscii++
  }
  return Math.ceil(ascii / 4) + nonAscii
}

// 一次回复里的「工作过程」：思考、模型中间说的话、工具调用
export interface ProcessItem {
  kind: 'reasoning' | 'text' | 'tool'
  content?: string
  name?: string
  toolId?: string
  callId?: string
  status?: 'run' | 'done' | 'fail'
  displayType?: string
  displayContent?: string
  requestId?: string
  answer?: string
  error?: string
}

export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  process?: ProcessItem[]
  reasoningEffort?: string
  reasoningTokens?: number
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
    // 对话还没创建时先攒着，createChat 成功后写进新会话（一次性）
    pendingToolConfig: null as ChatToolConfig | null,
    // pendingPersonaId 用于新建对话；已有对话的人设单独保存。
    personas: [] as Persona[],
    pendingPersonaId: '',
    workspaces: [] as Workspace[],
    workspacesLoaded: false,
    pendingWorkspaceId: '',
    workspaceChanging: false,
    toolConfigChanging: false,
    // 默认「中」，与后端 NewChatTask 的默认值一致
    effort: 'high' as 'none' | 'low' | 'high' | 'max',
  }),
  getters: {
    currentWorkspaceId(state): string {
      return state.session ? state.session.workspaceId ?? '' : state.pendingWorkspaceId
    },
    currentWorkspace(): Workspace | undefined {
      return this.workspaces.find((item) => item.id === this.currentWorkspaceId)
    },
    permissionLevel(state): PermissionLevel {
      const config = state.session ? state.session.toolConfig : state.pendingToolConfig
      return config?.permission || useConfigsStore().toolNormalConfig?.permission || 2
    },
    permission(): string {
      return { 1: '只读', 2: '自动编辑', 3: '自动审批', 4: '完全访问' }[this.permissionLevel]
    },
    ctxUsedTokens(state): number | null {
      if (state.usage) {
        return state.usage.total_tokens || state.usage.prompt_tokens + state.usage.completion_tokens
      }
      if (!state.messages.length) return null

      const root = state.session?.tree?.root
      const personaId = state.session ? root?.personaId || 'assistant' : state.pendingPersonaId
      const activePersona = state.personas.find((item) => item.id === personaId)
      let tokens = 3
      if (activePersona) {
        tokens += 4 + estimateTextTokens(JSON.stringify({
          name: activePersona.name,
          description: activePersona.description,
          system: activePersona.system,
        }))
      }

      let node = root?.next?.[root.choice ?? 0]
      const seen = new Set<string>()
      while (node && !seen.has(node.id)) {
        seen.add(node.id)
        for (const message of node.child ?? []) {
          tokens += 4 + estimateTextTokens(message.content ?? '') + estimateTextTokens(message.reasoning ?? '')
          for (const call of message.tool_calls ?? []) {
            tokens += estimateTextTokens(call.name) + estimateTextTokens(call.arguments)
          }
        }
        node = node.next?.[node.choice]
      }
      // 新发送的消息尚未写进树，临时消息也计入估算。
      for (const message of state.messages) {
        if (message.nodeId) continue
        tokens += 4 + estimateTextTokens(message.content)
        for (const item of message.process ?? []) {
          if (item.content) tokens += estimateTextTokens(item.content)
        }
      }
      return tokens
    },
    ctxPercent(state): number | null {
      const used = this.ctxUsedTokens
      if (used === null || !state.contextWindow) return null
      return Math.min(100, (used / state.contextWindow) * 100)
    },
    ctxTooltip(state): string {
      const tokens = this.ctxUsedTokens
      if (tokens === null) return '尚无上下文用量数据'
      const used = tokens.toLocaleString()
      if (!state.contextWindow) {
        return `${state.usage ? '本次请求' : '对话内容估算'} ${used} tokens · 模型上下文窗口未知`
      }
      const percent = this.ctxPercent ?? 0
      return `${state.usage ? '上下文约' : '上下文估算约'} ${percent.toFixed(1)}% · ${used} / ${state.contextWindow.toLocaleString()} tokens`
    },
  },
  actions: {
    reset() {
      ++contextWindowRequestId
      this.session = null
      this.messages = []
      this.draft = ''
      this.error = ''
      this.sending = false
      this.usage = null
      this.contextWindow = null
      // 新建对话回到默认档位，不再沿用上一个对话的设置
      this.effort = 'high'
    },
    // 从会话数据同步思考强度：切换对话、重新生成、改模型后都会走这里
    syncEffort() {
      const saved = this.session?.reasoningEffort
      // 老会话没有这个字段，按当前默认「中」处理
      this.effort = saved === 'none' || saved === 'low' || saved === 'high' || saved === 'max'
        ? saved
        : 'high'
    },
    // 用户在菜单里改档位：立即写回当前对话，切走再回来还能读回
    async setEffort(effort: 'none' | 'low' | 'high' | 'max') {
      if (this.sending) return
      this.effort = effort
      const id = this.session?.id
      if (!id) return
      // 按选择顺序写回，避免较早的请求后返回，把新档位覆盖成旧档位。
      const update = effortUpdates.then(async () => {
        const session = await SetChatReasoningEffort(id, effort)
        // 请求期间用户可能已经切到别的对话，别把旧会话盖回来
        if (this.session?.id === id && this.effort === effort) {
          this.session = session
          this.syncEffort()
        }
      })
      effortUpdates = update.catch(() => undefined)
      try {
        await update
      } catch (err) {
        this.error = String(err)
      }
    },
    async loadContextWindow() {
      const requestId = ++contextWindowRequestId
      this.contextWindow = null
      const id = this.session?.id
      if (!id) return
      try {
        const contextWindow = await GetChatModelContextWindow(id)
        if (requestId === contextWindowRequestId && this.session?.id === id) this.contextWindow = contextWindow
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
        // 思考内容直接进工作过程，作为其中一段
        last.process = last.process ?? []
        const tail = last.process[last.process.length - 1]
        if (tail?.kind === 'reasoning') {
          tail.content = (tail.content ?? '') + delta.reasoning
        } else {
          last.process.push({ kind: 'reasoning', content: delta.reasoning })
        }
      }
      if (delta.content) {
        last.content += delta.content
      }
    },
    applyTool(tool: ChatToolEvent) {
      if (tool.chatId && tool.chatId !== this.session?.id) return
      const last = this.messages[this.messages.length - 1]
      if (!last || last.role !== 'assistant') return
      last.process = last.process ?? []

      if (tool.event === 'start') {
        // 模型决定调工具时，它已经说出口的话属于「工作过程」，正文让给最终回复
        if (last.content.trim()) {
          last.process.push({ kind: 'text', content: last.content })
          last.content = ''
        }
        last.process.push({ kind: 'tool', toolId: tool.toolId, callId: tool.callId, name: tool.name, status: 'run',
          displayType: tool.displayType, displayContent: tool.displayContent, content: tool.content })
        return
      }

      for (let i = last.process.length - 1; i >= 0; i--) {
        const item = last.process[i]
        const matches = tool.callId ? item.callId === tool.callId
          : tool.toolId ? item.toolId === tool.toolId : item.name === tool.name
        if (item.kind === 'tool' && matches && item.status === 'run') {
          if (tool.content !== undefined) item.content = tool.content
          if (tool.displayType !== undefined) item.displayType = tool.displayType
          if (tool.displayContent !== undefined) item.displayContent = tool.displayContent
          if (tool.requestId !== undefined) item.requestId = tool.requestId
          if (tool.answer !== undefined) item.answer = tool.answer
          if (tool.error !== undefined) item.error = tool.error
          if (tool.event !== 'update') {
            item.status = tool.event === 'fail' ? 'fail' : 'done'
            item.requestId = undefined
          }
          return
        }
      }
    },
    async send() {
      const text = this.draft.trim()
      if (!text || this.sending || this.workspaceChanging) return
      const effort = this.effort
      // 首轮创建会话也属于发送过程，防止重复提交覆盖正在接收的思考。
      this.sending = true
      this.error = ''

      try {
        await effortUpdates
        await toolConfigUpdates
        if (!this.session) {
          await this.createChat(effort)
        }
        if (!this.session?.configId || !this.session.model) {
          this.error = '请先为此对话选择提供商和模型。'
          return
        }
        this.usage = null
        this.draft = ''
        this.messages.push({ role: 'user', content: text })
        this.messages.push({ role: 'assistant', content: '', reasoningEffort: effort })
        await SendChatMessage(this.session.id, text, effort)
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
      this.usage = null
      const root = session?.tree?.root
      const rootNodes = root?.next ?? []
      if (!root || root.choice < 0 || root.choice >= rootNodes.length) return
      let node = rootNodes[root.choice]
      let parentId = ''
      while (node) {
        this.usage = node.usage ?? null
        const siblings = parentId ? this.findNode(parentId)?.next ?? [] : rootNodes
        const branchIndex = parentId ? this.findNode(parentId)?.choice ?? 0 : root.choice
        const meta = { nodeId: node.id, parentId, branchIndex, branchCount: siblings.length }
        const children = node.child ?? []

        for (const message of children) {
          // 用户消息节点只有一条
          if (message.role === 'user') {
            this.messages.push({ role: 'user', content: message.content ?? '', ...meta })
          }
        }

        // 助手节点：child 里是 [assistant(可能带 tool_calls), tool, tool, assistant...]
        // 最后一条 assistant 是正式回复，之前的都归入「工作过程」
        const assistantChildren = children.filter((item) => item.role === 'assistant')
        if (assistantChildren.length) {
          const final = assistantChildren[assistantChildren.length - 1]
          const finalIndex = children.lastIndexOf(final)
          const process: ProcessItem[] = []

          for (let ci = 0; ci <= finalIndex; ci++) {
            const item = children[ci]
            if (item.role === 'tool') continue
            // 每轮思考单独成段：轮与轮之间会被工具调用隔开
            if (item.reasoning) process.push({ kind: 'reasoning', content: item.reasoning })
            if (ci < finalIndex && (item.content ?? '').trim()) process.push({ kind: 'text', content: item.content })
            for (const call of item.tool_calls ?? []) {
              const result = children.slice(ci + 1).find((entry) => entry.role === 'tool' && entry.tool_call_id === call.id)
              process.push({ kind: 'tool', toolId: call.name, callId: call.id, name: call.display_name || call.name,
                status: !result || result.tool_error ? 'fail' : 'done',
                displayType: call.display_type, displayContent: call.display_content,
                content: call.display_content === 'arguments' || call.display_content?.startsWith('arguments.') ? call.arguments : result?.content,
                answer: call.display_type === 'question' ? result?.content : undefined,
                error: result?.tool_error ?? (!result ? '调用未完成' : undefined),
              })
            }
          }
          this.messages.push({
            role: 'assistant',
            content: final.content ?? '',
            process: process.length ? process : undefined,
            reasoningEffort: node.reasoningEffort,
            reasoningTokens: node.usage?.completion_tokens_details?.reasoning_tokens,
            ...meta,
          })
        }

        parentId = node.id
        const nextNodes = node.next ?? []
        if (!nextNodes.length || node.choice < 0 || node.choice >= nextNodes.length) break
        node = nextNodes[node.choice]
      }
    },
    findNode(id: string) {
      const root = this.session?.tree?.root
      if (!root) return undefined
      const pending = [...(root.next ?? [])]
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
    startNewChat() {
      this.reset()
    },
    // 对话创建前在工具配置弹窗里改的设置，先存在这里
    setPendingToolConfig(config: ChatToolConfig | null) {
      this.pendingToolConfig = config
    },
    async updateToolConfig(config: ChatToolConfig | null) {
      if (this.sending || this.workspaceChanging || this.toolConfigChanging) return
      const id = this.session?.id
      if (!id) {
        this.pendingToolConfig = config
        return
      }
      this.toolConfigChanging = true
      const update = toolConfigUpdates.then(async () => {
        const session = await SetChatToolConfig(id, JSON.stringify(config ?? {}))
        if (this.session?.id === id) this.session = session
      })
      toolConfigUpdates = update.catch(() => undefined)
      try {
        await update
      } finally {
        this.toolConfigChanging = false
      }
    },
    async setPermission(permission: PermissionLevel) {
      const current = (this.session ? this.session.toolConfig : this.pendingToolConfig) ?? {}
      await this.updateToolConfig({ ...current, permission })
    },
    async loadWorkspaces() {
      this.workspaces = await ListWorkspaces()
      this.workspacesLoaded = true
    },
    async selectWorkspace(workspaceId: string) {
      if (this.sending || this.workspaceChanging) return
      this.workspaceChanging = true
      try {
        await toolConfigUpdates
        const id = this.session?.id
        if (id) {
          const session = await SetChatWorkspace(id, workspaceId)
          if (this.session?.id === id) this.session = session
          await this.loadChats()
        }
        this.pendingWorkspaceId = workspaceId
      } finally {
        this.workspaceChanging = false
      }
    },
    async createWorkspace(name: string, path: string) {
      const workspace = await CreateWorkspace(name, path)
      this.workspaces.push(workspace)
      this.workspacesLoaded = true
      await this.selectWorkspace(workspace.id)
      return workspace
    },
    async renameWorkspace(id: string, name: string) {
      const workspace = await RenameWorkspace(id, name)
      this.workspaces = this.workspaces.map((item) => item.id === id ? workspace : item)
    },
    async deleteWorkspace(id: string, deleteChats = false) {
      if (this.workspaceChanging) throw new Error('工作区正在切换中，请稍后重试')
      this.workspaceChanging = true
      try {
        await toolConfigUpdates
        if (deleteChats) await DeleteWorkspaceAndChats(id)
        else await DeleteWorkspace(id)
        this.workspaces = this.workspaces.filter((item) => item.id !== id)
        this.chats = deleteChats
          ? this.chats.filter((item) => item.workspaceId !== id)
          : this.chats.map((item) => item.workspaceId === id ? { ...item, workspaceId: '' } : item)
        if (this.pendingWorkspaceId === id) this.pendingWorkspaceId = ''
        if (this.session?.workspaceId === id) {
          if (deleteChats) this.reset()
          else this.session = { ...this.session, workspaceId: '' }
        }
      } finally {
        this.workspaceChanging = false
      }
    },
    setPendingPersona(id: string) {
      this.pendingPersonaId = id
    },
    async updatePersona(id: string, personaId: string) {
      const session = await SetChatPersona(id, personaId)
      if (this.session?.id === id) {
        this.session = session
        this.syncEffort()
      }
      await this.loadChats()
    },
    async loadPersonas() {
      if (this.personas.length) return
      this.personas = await ListPersonas()
      if (!this.pendingPersonaId) this.pendingPersonaId = this.personas[0]?.id ?? ''
    },
    async createChat(effort = this.effort) {
      const configs = useConfigsStore()
      const session = await CreateChat(
        configs.enables?.ID ?? '',
        configs.enabledModel,
        '',
        this.pendingPersonaId,
      )
      this.session = session
      this.rebuildMessages()
      if (this.pendingWorkspaceId) {
        this.session = await SetChatWorkspace(session.id, this.pendingWorkspaceId)
      }
      // 后端新会话默认就是 high，只有用户改过档位才需要写回
      if (effort !== 'high') {
        this.session = await SetChatReasoningEffort(session.id, effort)
      }
      // 把对话开始前攒下的工具配置写进新会话，写完即清空（只作用于这一次）
      if (this.pendingToolConfig) {
        const pending = this.pendingToolConfig
        this.session = await SetChatToolConfig(session.id, JSON.stringify(pending))
        this.pendingToolConfig = null
      }
      await this.loadChats()
      return this.session
    },
    async openChat(id: string) {
      this.error = ''
      await toolConfigUpdates
      this.session = await GetChat(id)
      this.pendingWorkspaceId = this.session.workspaceId ?? ''
      this.syncEffort()
      this.rebuildMessages()
    },
    async reloadSession() {
      if (!this.session) return
      this.session = await GetChat(this.session.id)
      this.syncEffort()
      this.rebuildMessages()
      await this.loadChats()
    },
    async deleteChat(id: string) {
      await DeleteChat(id)
      this.chats = this.chats.filter((chat) => chat.id !== id)
      if (this.session?.id === id) this.reset()
    },
    async renameChat(id: string, title: string) {
      const session = await RenameChat(id, title)
      if (this.session?.id === id) {
        this.session = session
        this.syncEffort()
      }
      await this.loadChats()
    },
    async selectBranch(parentId: string, choice: number) {
      if (!this.session) return
      this.session = await SetChatBranch(this.session.id, parentId, choice)
      this.syncEffort()
      this.rebuildMessages()
    },
    async updateModel(configId: string, model: string) {
      if (!this.session) return
      this.session = await UpdateChatModel(this.session.id, configId, model)
      this.syncEffort()
      await this.loadContextWindow()
      await this.loadChats()
    },
    // 编辑用户消息：后端在该节点的父节点下挂新分支并重新生成，原分支保留
    async editMessage(message: ChatMessage, text: string) {
      if (!this.session || !message.nodeId || message.role !== 'user' || this.sending) return
      text = text.trim()
      if (!text || text === message.content) return
      const index = this.messages.indexOf(message)
      if (index < 0) return
      const effort = this.effort
      // 新分支先替换可见的后续消息，让流式事件写入空回复；原分支留在会话树中。
      this.messages = [
        ...this.messages.slice(0, index),
        { role: 'user', content: text },
        { role: 'assistant', content: '', reasoningEffort: effort },
      ]
      this.usage = null
      this.error = ''
      this.sending = true
      try {
        await EditChatMessage(this.session.id, message.nodeId, text, effort)
        await this.reloadSession()
      } catch (err) {
        this.error = String(err)
        await this.reloadSession().catch(() => undefined)
      } finally {
        this.sending = false
      }
    },
    async regenerate(message: ChatMessage) {
      if (!this.session || !message.nodeId || message.role !== 'assistant' || this.sending) return
      const index = this.messages.indexOf(message)
      this.messages = this.messages.slice(0, index + 1)
      const pending = this.messages[index]
      pending.content = ''
      pending.process = []
      pending.reasoningEffort = this.effort
      pending.reasoningTokens = undefined
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
