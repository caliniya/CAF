import { defineStore } from 'pinia'
import {
  CreateChat, CompactChat, SetChatCompactionMode, SubmitCompactionDecision, GetPendingCompaction,
  type SearchReceipt,
  builtInCompaction, type CompactionMode, type CompactionPrompt, type ChatTreeNode,
  CreateMemoryLibrary,
  CreateWorkspace,
  DeleteChat,
  DeleteMemoryLibrary,
  DeleteWorkspace,
  DeleteWorkspaceAndChats,
  EditChatMessage,
  GetChatModelContextWindow,
  GetChat,
  GetChatMemorySettings,
  GetWorkspaceMemorySettings,
  ListChats,
  ListPersonas,
  ListWorkspaces,
  RenameChat,
  RenameMemoryLibrary,
  RenameWorkspace,
  RegenerateChatMessage,
  SendChatMessage, StopChatGeneration, type ChatAttachment, type AttachmentUpload,
  SetChatBranch,
  SetChatToolChangesUndone,
  SetChatPersona,
  SetChatReasoningEffort,
  SetChatToolConfig,
  SetChatMemoryLibraries,
  SetMemoryLibraryScope,
  SetChatWorkspace,
  UpdateChatModel,
  type ChatTask as StoredChatTask,
  type ChatMemorySettings,
  type MemoryLibraryAction,
  type ChatSummary,
  type ChatToolConfig,
  type ChatDeltaEvent,
  type ChatToolEvent,
  type Persona,
  type TokenUsage,
  type Workspace,
  type PermissionLevel,
  type ToolApproval,
  type ToolOperation,
  imageInputState, type Model, type Config,
} from '../api'
import { useConfigsStore } from './configs'

let openChatRequest = 0
let contextWindowRequestId = 0
let compactionUpdates: Promise<void> = Promise.resolve()
let effortUpdates: Promise<void> = Promise.resolve()
let toolConfigUpdates: Promise<void> = Promise.resolve()
let memorySettingsUpdates: Promise<void> = Promise.resolve()

// 没有历史 token 记录时粗略估算：ASCII 约四个字符一 token，其余字符约两个 token，与后端估算相近。
function estimateTextTokens(text: string): number {
  let ascii = 0
  let nonAscii = 0
  for (const char of text) {
    if (char.codePointAt(0)! <= 0x7f) ascii++
    else nonAscii++
  }
  return Math.ceil(ascii / 4) + nonAscii * 2
}

const textTypes = '.txt,.md,.log,.json,.csv,.go,.py,.js,.ts,.vue,.yaml,.yml,.toml,.xml,.html,.css,.rs,.c,.cpp,.h,.java,.sh,.ps1,.ini'
const allTypes = '.png,.jpg,.jpeg,' + textTypes
const isImageFile = (file: File) => file.type.startsWith('image/') || /\.(png|jpe?g)$/i.test(file.name)
async function uploadFile(file: File): Promise<AttachmentUpload> {
  const data = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(new Error('读取附件失败：' + file.name))
    reader.onload = () => resolve(String(reader.result).split(',')[1] ?? '')
    reader.readAsDataURL(file)
  })
  return {name: file.name, data}
}
function directImage(image: ChatAttachment): ProcessItem {
  return {kind: 'tool', name: '读取图片', displayType: 'image', image, imageSent: true, status: 'done'}
}

function mergeWebSearch(current: SearchReceipt | undefined, incoming?: SearchReceipt | null): SearchReceipt | undefined {
  if (!incoming) return current
  const sources = new Map((current?.sources ?? []).map(s => [s.url, s]))
  for (const source of incoming.sources ?? []) {
    try {
      const url = new URL(source.url)
      if (url.protocol !== 'http:' && url.protocol !== 'https:' || url.username || url.password) continue
      if (!sources.has(source.url) || source.title) sources.set(source.url, source)
    } catch { /* ignore unusable citation URLs */ }
  }
  return { sources: [...sources.values()] }
}

// 一次回复里的「工作过程」：思考、模型中间说的话、工具调用
export interface ProcessItem {
  kind: 'reasoning' | 'text' | 'tool'
  content?: string
  name?: string
  toolId?: string
  callId?: string
  status?: 'run' | 'done' | 'fail'
  image?: ChatAttachment
  imageSent?: boolean
  displayType?: string
  displayContent?: string
  requestId?: string
  answer?: string
  error?: string
  approval?: ToolApproval
}

export interface ChatMessage {
  webSearchOffered?: boolean
  webSearch?: SearchReceipt
  role: 'user' | 'assistant' | 'system'
  content: string
  stopped?: boolean
  attachments?: ChatAttachment[]
  process?: ProcessItem[]
  reasoningEffort?: string
  reasoningTokens?: number
  nodeId?: string
  parentId?: string
  branchIndex?: number
  branchCount?: number
  toolOperations?: ToolOperation[]
  canUndoToolChanges?: boolean
  canRedoToolChanges?: boolean
}

export const useChatStore = defineStore('chat', {
  state: () => ({
    chats: [] as ChatSummary[],
    session: null as StoredChatTask | null,
    messages: [] as ChatMessage[],
    draft: '',
    draftFiles: [] as File[],
    sending: false,
    generation: null as {chatId: string; nodeId: string} | null,
    stopping: false,
    error: '',
    usage: null as TokenUsage | null,
    contextWindow: null as number | null,
    // 对话还没创建时先攒着，createChat 成功后写进新会话（一次性）
    pendingToolConfig: null as ChatToolConfig | null,
    pendingMemoryLibraryIds: null as string[] | null,
    pendingCompactionMode: null as CompactionMode | null,
    compactionChanging: false,
    compactionPrompts: {} as Record<string, CompactionPrompt>,
    answeringCompaction: false,
    compactionStatus: '',
    // pendingPersonaId 用于新建对话；已有对话的人设单独保存。
    personas: [] as Persona[],
    pendingPersonaId: '',
    workspaces: [] as Workspace[],
    workspacesLoaded: false,
    pendingWorkspaceId: '',
    workspaceChanging: false,
    toolConfigChanging: false,
    toolChangesChanging: false,
    // 默认「中」，与后端 NewChatTask 的默认值一致
    effort: 'high' as 'none' | 'low' | 'high' | 'max',
  }),
  getters: {
    composerConfig(state): Config | null {
      const configs = useConfigsStore()
      const session = state.session
      return session ? configs.configs.find(c => c.ID === session.configId) ?? null : configs.enables
    },
    composerModel(): Model | undefined {
      const config = this.composerConfig
      const id = this.session ? this.session.model : config?.EnabledModel
      return config?.Model?.find(m => m.id === id)
    },
    imageInputIssue(): string {
      switch (imageInputState(this.composerModel)) {
        case 'unsupported': return '当前模型不支持图片输入，请切换到带「视觉」标签的模型。'
        case 'unknown': return '尚未确认当前模型支持图片输入，可在模型供应商设置中确认视觉能力，或切换到带「视觉」标签的模型。'
        default: return ''
      }
    },
    attachmentTypes(): string { return this.imageInputIssue ? textTypes : allTypes },
    draftImageIssue(): string { return this.draftFiles.some(isImageFile) ? this.imageInputIssue : '' },
    generationNode(state): string {
      return state.generation && state.generation.chatId === state.session?.id ? state.generation.nodeId : ''
    },
    enabledMemoryLibraryIds(state): string[] {
      return state.session ? state.session.memoryLibraryIds ?? [] : state.pendingMemoryLibraryIds ?? useConfigsStore().defaultMemoryIds ?? []
    },
    compactionMode(state): CompactionMode {
      return (state.session ? state.session.compactionMode : state.pendingCompactionMode) ?? useConfigsStore().defaultCompaction ?? builtInCompaction
    },
    compactionPrompt(state): CompactionPrompt | undefined {
      return state.session ? state.compactionPrompts[state.session.id] : undefined
    },
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
    strictMode(state): boolean {
      const config = state.session ? state.session.toolConfig : state.pendingToolConfig
      return config?.strictMode ?? useConfigsStore().toolNormalConfig?.strictMode ?? false
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
      const path: ChatTreeNode[] = []
      while (node && !seen.has(node.id)) { seen.add(node.id); path.push(node); node = node.next?.[node.choice] }
      const checkpoint = [...path].reverse().find(n => n.compaction)?.compaction
      const start = checkpoint ? path.findIndex(n => n.id === checkpoint.start.nodeId) : 0
      if (checkpoint) tokens += estimateTextTokens(checkpoint.summary + JSON.stringify(checkpoint.results ?? []))
      for (const item of path.slice(Math.max(start, 0))) {
        const offset = checkpoint?.start.nodeId === item.id ? checkpoint.start.index : 0
        for (const message of (item.child ?? []).slice(offset)) {
          if (message.tokens) { tokens += message.tokens; continue }
          tokens += 4 + estimateTextTokens(message.content ?? '') + estimateTextTokens(message.reasoning ?? '')
          for (const call of message.tool_calls ?? []) tokens += estimateTextTokens(call.name) + estimateTextTokens(call.arguments)
        }
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
    applyMemorySettings(settings: ChatMemorySettings, chatId: string, workspaceId: string): ChatMemorySettings {
      if ((this.session?.id ?? '') !== chatId || this.currentWorkspaceId !== workspaceId) return settings
      if (this.session) {
        this.session.memoryLibraryIds = settings.enabledIds
      } else if (this.pendingMemoryLibraryIds !== null) {
        this.pendingMemoryLibraryIds = this.pendingMemoryLibraryIds.filter(key => settings.libraries.some(library => library.id === key))
      } else {
        useConfigsStore().defaultMemoryIds = settings.enabledIds
      }
      return { ...settings, enabledIds: [...this.enabledMemoryLibraryIds] }
    },
    async loadMemorySettings() {
      const id = this.session?.id ?? ''
      const workspaceId = this.currentWorkspaceId
      const settings = id ? await GetChatMemorySettings(id) : await GetWorkspaceMemorySettings(workspaceId)
      return this.applyMemorySettings(settings, id, workspaceId)
    },
    async manageMemoryLibrary(action: MemoryLibraryAction) {
      if (this.sending || this.workspaceChanging) throw new Error('对话正在生成或切换工作区，暂时无法管理记忆')
      const chatId = this.session?.id ?? ''
      const workspaceId = this.currentWorkspaceId
      const update = memorySettingsUpdates.then(async () => {
        switch (action.kind) {
          case 'create': await CreateMemoryLibrary(chatId, workspaceId, action.name, action.scope); break
          case 'rename': await RenameMemoryLibrary(chatId, workspaceId, action.id, action.name); break
          case 'delete': await DeleteMemoryLibrary(chatId, workspaceId, action.id); break
          case 'scope': await SetMemoryLibraryScope(chatId, workspaceId, action.id, action.scope); break
        }
        const settings = chatId ? await GetChatMemorySettings(chatId) : await GetWorkspaceMemorySettings(workspaceId)
        return this.applyMemorySettings(settings, chatId, workspaceId)
      })
      memorySettingsUpdates = update.then(() => undefined, () => undefined)
      return await update
    },
    async setMemoryLibraries(ids: string[]) {
      if (this.sending || this.workspaceChanging) throw new Error('对话正在生成中或切换工作区，暂时无法修改记忆设置')
      const id = this.session?.id
      if (!id) {
        this.pendingMemoryLibraryIds = [...new Set(ids)]
        return
      }
      const update = memorySettingsUpdates.then(async () => {
        const session = await SetChatMemoryLibraries(id, ids)
        if (this.session?.id === id) this.session.memoryLibraryIds = session.memoryLibraryIds ?? []
      })
      memorySettingsUpdates = update.catch(() => undefined)
      await update
    },
    reset() {
      ++openChatRequest
      ++contextWindowRequestId
      this.session = null
      this.messages = []
      this.draft = ''
      this.draftFiles = []
      this.error = ''
      this.sending = false; this.generation = null; this.stopping = false
      this.usage = null
      this.contextWindow = null
      // 新建对话回到默认档位，不再沿用上一个对话的设置
      this.effort = 'high'
      this.pendingCompactionMode = null
      this.compactionStatus = ''
      this.pendingMemoryLibraryIds = null
    },
    // 从会话数据同步思考强度：切换对话、重新生成、改模型后都会走这里
    syncEffort() {
      const saved = this.session?.reasoningEffort
      // 老会话没有这个字段，按当前默认「中」处理
      this.effort = saved === 'none' || saved === 'low' || saved === 'high' || saved === 'max'
        ? saved
        : 'high'
    },
    async setCompactionMode(mode: CompactionMode | null) {
      if (this.sending || this.compactionChanging) return
      const id = this.session?.id
      if (!id) { this.pendingCompactionMode = mode; return }
      this.compactionChanging = true
      const update = compactionUpdates.then(async () => {
        const session = await SetChatCompactionMode(id, mode)
        if (this.session?.id === id) this.session.compactionMode = session.compactionMode
      })
      compactionUpdates = update.catch(() => undefined)
      try { await update } finally { this.compactionChanging = false }
    },
    applyCompactionPrompt(prompt: CompactionPrompt) {
      if (!prompt.chatId || !prompt.requestId) return
      if (!prompt.closed) this.compactionPrompts[prompt.chatId] = prompt
      else if (this.compactionPrompts[prompt.chatId]?.requestId === prompt.requestId) delete this.compactionPrompts[prompt.chatId]
    },
    async answerCompaction(compact: boolean) {
      const prompt = this.compactionPrompt
      if (!prompt || this.answeringCompaction) return
      this.answeringCompaction = true
      try {
        await SubmitCompactionDecision(prompt.chatId, prompt.requestId, compact)
        this.applyCompactionPrompt({ ...prompt, closed: true })
      } catch (err) { this.error = String(err) }
      finally { this.answeringCompaction = false }
    },
    async compactNow() {
      const id = this.session?.id
      if (!id || this.sending || this.compactionChanging || this.toolChangesChanging || this.workspaceChanging || this.compactionPrompt) return
      this.sending = true; this.error = ''; this.compactionStatus = '正在压缩上下文…'
      try {
        await Promise.all([compactionUpdates, effortUpdates, toolConfigUpdates, memorySettingsUpdates])
        const session = await CompactChat(id)
        if (this.session?.id !== id) return
        this.session = session; this.rebuildMessages(); this.usage = null; this.compactionStatus = '上下文已压缩，原始消息仍然保留。'
        await this.loadChats()
      } catch (err) { if (this.session?.id === id) { this.error = String(err); this.compactionStatus = '' } }
      finally { this.sending = false; this.generation = null; this.stopping = false }
    },
    // 用户在菜单里改档位：立即写回当前对话，切走再回来还能读回。
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
    generationStarted(event: {chatId: string; nodeId: string}) {
      if (this.sending && this.session?.id === event.chatId) this.generation = event
    },
    async stopGeneration() {
      const node = this.generationNode, id = this.session?.id
      if (!id || !node || this.stopping) return
      this.stopping = true
      try { await StopChatGeneration(id, node) }
      catch (e) {
        if (this.generation?.nodeId === node) {
          this.stopping = false
          if (this.session?.id === id) this.error = String(e)
        }
      }
    },
    applyDelta(delta: ChatDeltaEvent) {
      if (delta.chatId && delta.chatId !== this.session?.id) return
      if (delta.usage) this.usage = delta.usage
      const last = this.messages[this.messages.length - 1]
      if (!last || last.role !== 'assistant') return
      if (delta.webSearch) last.webSearch = mergeWebSearch(last.webSearch, delta.webSearch)
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

      if (tool.event === 'image' && tool.image) {
        if (tool.callId) {
          const item = last.process.find(p => p.kind === 'tool' && p.callId === tool.callId)
          if (item) { item.image = tool.image; item.imageSent = true }
        } else if (!last.process.some(p => !p.callId && p.image?.id === tool.image?.id)) last.process.push(directImage(tool.image))
        return
      }
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
          if (tool.approval !== undefined) {
            try { item.approval = typeof tool.approval === 'string' ? JSON.parse(tool.approval) : tool.approval } catch { /* 忽略无效事件数据 */ }
          }
          if (tool.event !== 'update') {
            item.status = tool.event === 'fail' ? 'fail' : 'done'
            item.requestId = undefined
          }
          return
        }
      }
    },
    addFiles(files: File[]) {
      if (this.sending || !files.length) return
      const blocked = files.some(isImageFile) ? this.imageInputIssue : ''
      if (blocked) files = files.filter(file => !isImageFile(file))
      const next = [...this.draftFiles, ...files]
      const extensions = allTypes.split(',')
      for (const file of files) {
        const ext = file.name.toLowerCase().match(/\.[^.]+$/)?.[0] ?? ''
        if (!extensions.includes(ext)) { this.error = '暂不支持此附件类型：' + file.name; return }
        const image = isImageFile(file)
        if (file.size > (image ? 8 * 1024 * 1024 : 256 * 1024)) { this.error = image ? '单张图片不能超过 8 MiB' : '单份文本附件不能超过 256 KiB'; return }
      }
      if (next.length > 4 || next.reduce((n, f) => n + f.size, 0) > 12 * 1024 * 1024) { this.error = '每条消息最多 4 个附件，合计最多 12 MiB'; return }
      this.error = blocked; this.draftFiles = next
    },
    async send() {
      this.compactionStatus = ''
      const text = this.draft.trim()
      const view = openChatRequest
      const files = [...this.draftFiles], startingChat = this.session?.id
      if ((!text && !files.length) || this.compactionPrompt || this.sending || this.workspaceChanging || this.toolChangesChanging) return
      if (this.draftImageIssue) { this.error = this.draftImageIssue; return }
      const effort = this.effort
      let sentTo = '', previousLastUser: string | undefined
      // 首轮创建会话也属于发送过程，防止重复提交覆盖正在接收的思考。
      this.sending = true
      this.error = ''

      try {
        await compactionUpdates
        await effortUpdates
        await toolConfigUpdates
        await memorySettingsUpdates
        if (view !== openChatRequest || startingChat !== this.session?.id) return
        if (!this.session) {
          await this.createChat(effort)
          if (view !== openChatRequest) return
        }
        if (!this.session?.configId || !this.session.model) {
          this.error = '请先为此对话选择提供商和模型。'
          return
        }
        const target = this.session.id
        const withImages = files.some(isImageFile)
        if (withImages && this.imageInputIssue) { this.error = this.imageInputIssue; return }
        const uploads = await Promise.all(files.map(uploadFile))
        if (target !== this.session?.id) return
        if (withImages && this.imageInputIssue) { this.error = this.imageInputIssue; return }
        this.usage = null
        sentTo = this.session.id; previousLastUser = this.messages.filter(m => m.role === 'user').at(-1)?.nodeId
        this.draft = ''; this.draftFiles = []
        this.messages.push({ role: 'user', content: text, attachments: files.map(f => ({id:'', name:f.name, mime:f.type})) })
        this.messages.push({ role: 'assistant', content: '', reasoningEffort: effort })
        await SendChatMessage(sentTo, text, effort, uploads)
        await this.reloadSession()
      } catch (err) {
        this.error = String(err)
        await this.reloadSession().catch(() => undefined)
        if (sentTo && sentTo === this.session?.id && !this.draft && !this.draftFiles.length &&
          this.messages.filter(m => m.role === 'user').at(-1)?.nodeId === previousLastUser) {
          this.draft = text; this.draftFiles = files
        }
      } finally {
        this.sending = false; this.generation = null; this.stopping = false
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
          if (message.role === 'user' || message.role === 'system') {
            this.messages.push({ role: message.role, content: message.content ?? '', attachments: message.attachments, ...meta })
          }
        }

        // 助手节点：child 里是 [assistant(可能带 tool_calls), tool, tool, assistant...]
        // 最后一条 assistant 是正式回复，之前的都归入「工作过程」
        const assistantChildren = children.filter((item) => item.role === 'assistant')
        if (assistantChildren.length || node.inputImages?.length || node.stopped) {
          const final = assistantChildren[assistantChildren.length - 1] ?? {role:'assistant', content:''}
          const finalIndex = children.lastIndexOf(final)
          const process: ProcessItem[] = (node.inputImages ?? []).map(directImage)

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
                approval: call.approval, image: call.image, imageSent: call.image_sent,
                error: result?.tool_error ?? (!result ? '调用未完成' : undefined),
              })
            }
          }
          this.messages.push({
            role: 'assistant',
            content: final.content ?? '',
            webSearchOffered: node.webSearchOffered,
            webSearch: assistantChildren.reduce<SearchReceipt | undefined>((seen, m) => mergeWebSearch(seen, m.webSearch), undefined),
            process: process.length ? process : undefined,
            stopped: node.stopped, reasoningEffort: node.reasoningEffort,
            reasoningTokens: node.usage?.completion_tokens_details?.reasoning_tokens,
            toolOperations: node.toolOperations?.filter((operation) => !operation.pending),
            ...meta,
          })
        }

        parentId = node.id
        const nextNodes = node.next ?? []
        if (!nextNodes.length || node.choice < 0 || node.choice >= nextNodes.length) break
        node = nextNodes[node.choice]
      }
      const replies = this.messages.filter((message) => message.role === 'assistant' && message.toolOperations?.length)
      let lastApplied = -1
      const firstUndone = replies.findIndex((message) => message.toolOperations!.some((operation) => operation.state === 'undone'))
      for (let i = 0; i < replies.length; i++) {
        if (replies[i].toolOperations!.some((operation) => !operation.state)) lastApplied = i
      }
      for (let i = 0; i < replies.length; i++) {
        const message = replies[i]
        message.canUndoToolChanges = i === lastApplied && message.toolOperations!.every((operation) => !operation.state)
        message.canRedoToolChanges = i === firstUndone && i > lastApplied
          && message.toolOperations!.every((operation) => operation.state === 'undone')
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
      if (this.sending || this.workspaceChanging || this.toolConfigChanging || this.toolChangesChanging) return
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
      const next = { ...current, permission }
      if (permission < this.permissionLevel) delete next.allowedTools
      await this.updateToolConfig(next)
    },
    async setStrictMode(strictMode: boolean) {
      const current = (this.session ? this.session.toolConfig : this.pendingToolConfig) ?? {}
      await this.updateToolConfig({ ...current, strictMode })
    },
    async clearToolGrants() {
      const current = { ...(this.session ? this.session.toolConfig : this.pendingToolConfig) }
      delete current.allowedTools
      await this.updateToolConfig(current)
    },
    applyApprovalConfig({ chatId, config }: { chatId: string; config: ChatToolConfig | null }) {
      if (this.session?.id === chatId) this.session.toolConfig = config ?? undefined
    },
    async loadWorkspaces() {
      this.workspaces = await ListWorkspaces()
      this.workspacesLoaded = true
    },
    async selectWorkspace(workspaceId: string) {
      if (this.sending || this.workspaceChanging || this.toolChangesChanging || workspaceId === this.currentWorkspaceId) return
      const id = this.session?.id
      this.workspaceChanging = true
      try {
        await compactionUpdates
        await effortUpdates
        await toolConfigUpdates
        await memorySettingsUpdates
        if (id) {
          const session = await SetChatWorkspace(id, workspaceId)
          if (this.session?.id === id) {
            this.session = session
            this.pendingWorkspaceId = workspaceId
            this.rebuildMessages()
          }
          await this.loadChats()
        } else {
          const settings = await GetWorkspaceMemorySettings(workspaceId)
          if (!this.session) {
            this.pendingWorkspaceId = workspaceId
            this.applyMemorySettings(settings, '', workspaceId)
          }
        }
      } finally {
        this.workspaceChanging = false
      }
    },
    async createWorkspace(name: string, path: string) {
      const workspace = await CreateWorkspace(name, path)
      this.workspaces.push(workspace)
      this.workspacesLoaded = true
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
    async createChat(effort?: 'none' | 'low' | 'high' | 'max') {
      effort ??= this.effort
      const view = openChatRequest
      const configs = useConfigsStore()
      const session = await CreateChat(
        configs.enables?.ID ?? '',
        configs.enabledModel,
        '',
        this.pendingPersonaId,
      )
      if (view !== openChatRequest) return
      this.session = session
      this.rebuildMessages()
      if (this.pendingWorkspaceId) {
        this.session = await SetChatWorkspace(session.id, this.pendingWorkspaceId)
      }
      if (this.pendingMemoryLibraryIds !== null) {
        this.session = await SetChatMemoryLibraries(session.id, this.pendingMemoryLibraryIds)
      }
      this.pendingMemoryLibraryIds = null
      if (this.pendingCompactionMode) {
        this.session = await SetChatCompactionMode(session.id, this.pendingCompactionMode)
        this.pendingCompactionMode = null
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
      const request = ++openChatRequest
      this.compactionStatus = ''
      this.error = ''
      await Promise.all([compactionUpdates, toolConfigUpdates, memorySettingsUpdates])
      const [session, prompt] = await Promise.all([GetChat(id), GetPendingCompaction(id)])
      if (request !== openChatRequest) return
      if (this.session?.id !== id) this.draftFiles = []
      this.session = session
      if (prompt) this.applyCompactionPrompt(prompt)
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
      if (!this.session || this.sending || this.workspaceChanging || this.toolChangesChanging) return
      this.session = await SetChatBranch(this.session.id, parentId, choice)
      this.syncEffort()
      this.rebuildMessages()
    },
    async setToolChangesUndone(message: ChatMessage, undone: boolean) {
      if (!this.session || !message.nodeId || this.sending || this.workspaceChanging || this.toolChangesChanging) return
      if (undone ? !message.canUndoToolChanges : !message.canRedoToolChanges) return
      const id = this.session.id
      this.toolChangesChanging = true
      this.error = ''
      try {
        await compactionUpdates
        await effortUpdates
        await toolConfigUpdates
        await memorySettingsUpdates
        const session = await SetChatToolChangesUndone(id, message.nodeId, undone)
        if (this.session?.id === id) {
          this.session = session
          this.rebuildMessages()
        }
        await this.loadChats()
      } catch (err) {
        if (this.session?.id === id) this.error = String(err)
      } finally {
        this.toolChangesChanging = false
      }
    },
    async updateModel(configId: string, model: string) {
      if (!this.session) return
      const id = this.session.id
      const saved = await UpdateChatModel(id, configId, model)
      if (this.session?.id !== id) return
      this.session = saved
      this.syncEffort()
      await this.loadContextWindow()
      await this.loadChats()
    },
    // 编辑用户消息：后端在该节点的父节点下挂新分支并重新生成，原分支保留
    async editMessage(message: ChatMessage, text: string) {
      if (!this.session || !message.nodeId || message.role !== 'user' || this.sending || this.toolChangesChanging) return
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
        this.sending = false; this.generation = null; this.stopping = false
      }
    },
    async regenerate(message: ChatMessage) {
      if (!this.session || !message.nodeId || message.role !== 'assistant' || this.sending || this.toolChangesChanging) return
      const index = this.messages.indexOf(message)
      this.messages = this.messages.slice(0, index + 1)
      const pending = this.messages[index]
      pending.content = ''; pending.stopped = false
      pending.process = []
      pending.webSearchOffered = undefined
      pending.webSearch = undefined
      pending.reasoningEffort = this.effort
      pending.reasoningTokens = undefined
      pending.toolOperations = undefined
      pending.canUndoToolChanges = undefined
      pending.canRedoToolChanges = undefined
      this.error = ''
      this.sending = true
      try {
        await RegenerateChatMessage(this.session.id, message.nodeId, this.effort)
        await this.reloadSession()
      } catch (err) {
        this.error = String(err)
        await this.reloadSession().catch(() => undefined)
      } finally {
        this.sending = false; this.generation = null; this.stopping = false
      }
    },
  },
})
