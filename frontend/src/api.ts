// 与 Go 侧 Backend.Configs / Backend.Config 对应的 JSON 结构。
export interface Config {
  ID: string
  Name: string
  ServerAddress: string
  Protocol: 'chat-completions'
  APIKey: string
  Model: Model[]
  // 该配置自己记住的启用模型；每个提供商独立记忆，切换提供商自动恢复。
  EnabledModel: string
}

export interface Model {
  id: string
  name?: string
  context_window?: number
  max_output_tokens?: number
  input_modalities?: string[]
  output_modalities?: string[]
  supported_levels?: string[]
  default_level?: string
}

export interface Configs {
  config: Config[] | null
  EnablesConfig: Config | null
  toolNormalConfig?: ChatToolConfig
}

interface CoreBindings {
	GetClientExtensionChatConfig(clientId: string, extensionId: string): Promise<ExtensionChatConfig>
	UpdateClientExtensionChatConfig(clientId: string, extensionId: string, config: ExtensionChatConfig): Promise<ExtensionChatConfig>
	ListQQContexts(clientId: string, extensionId: string): Promise<QQContextInfo[]>
	ClearQQContext(clientId: string, extensionId: string, contextId: string): Promise<void>
	DeleteQQContext(clientId: string, extensionId: string, contextId: string): Promise<void>
  GetChatMemorySettings(id: string): Promise<ChatMemorySettings>
  GetWorkspaceMemorySettings(workspaceId: string): Promise<ChatMemorySettings>
  GetMemoryLibraryDetails(chatId: string, workspaceId: string, id: string): Promise<MemoryLibraryDetails>
  CreateMemoryEntry(chatId: string, workspaceId: string, libraryId: string, input: MemoryEntryInput): Promise<MemoryLibraryDetails>
  UpdateMemoryEntry(chatId: string, workspaceId: string, libraryId: string, memoryId: string, input: MemoryEntryInput): Promise<MemoryLibraryDetails>
  DeleteMemoryEntry(chatId: string, workspaceId: string, libraryId: string, memoryId: string): Promise<MemoryLibraryDetails>
  SetChatMemoryLibraries(id: string, ids: string[]): Promise<ChatTask>
  CreateMemoryLibrary(chatId: string, workspaceId: string, name: string, scope: MemoryLibrary['scope']): Promise<MemoryLibrary>
  RenameMemoryLibrary(chatId: string, workspaceId: string, id: string, name: string): Promise<void>
  DeleteMemoryLibrary(chatId: string, workspaceId: string, id: string): Promise<void>
  SetMemoryLibraryScope(chatId: string, workspaceId: string, id: string, scope: MemoryLibrary['scope']): Promise<void>
  ListClientExtensions(clientId: string): Promise<ExtensionOption[]>
  CreateClientExtension(clientId: string, type: string, parameters: Record<string, string>): Promise<ExtensionOption>
  GetClientExtensionParameters(clientId: string, id: string): Promise<Record<string, string>>
  UpdateClientExtensionParameters(clientId: string, id: string, parameters: Record<string, string>): Promise<void>
  DeleteClientExtension(clientId: string, id: string): Promise<void>
  ListExtensionTypes(): Promise<ExtensionDescriptor[]>
  SetClientExtensionEnabled(clientId: string, extensionId: string, enabled: boolean): Promise<ExtensionClient>
  ListExtensionClients(): Promise<ExtensionClient[]>
  GetExtensionClient(id: string): Promise<ExtensionClient>
  CreateExtensionClient(name: string, id: string): Promise<ExtensionClient>
  RenameExtensionClient(id: string, name: string): Promise<ExtensionClient>
  DeleteExtensionClient(id: string): Promise<void>
  SetExtensionClientQQ(id: string, adapterId: string, name: string, enabled: boolean): Promise<ExtensionClient>
  RemoveExtensionClientQQ(id: string, adapterId: string): Promise<ExtensionClient>
  SelectDirectory(defaultDirectory: string): Promise<string>
  ListWorkspaces(): Promise<Workspace[]>
  CreateWorkspace(name: string, path: string): Promise<Workspace>
  RenameWorkspace(id: string, name: string): Promise<Workspace>
  DeleteWorkspace(id: string): Promise<void>
  DeleteWorkspaceAndChats(id: string): Promise<void>
  SetChatWorkspace(id: string, workspaceId: string): Promise<ChatTask>
  LoadConfigs(): Promise<Configs>
  GetChatModelContextWindow(id: string): Promise<number>
  ProbeModels(serverAddress: string, apiKey: string): Promise<ModelProbeResult>
  AddModelConfig(name: string, serverAddress: string, protocol: Config['Protocol'], apiKey: string, models: Model[]): Promise<Configs>
  RefreshModelConfig(index: number, serverAddress: string, models: Model[]): Promise<Configs>
  SelectConfig(index: number): Promise<Configs>
  SelectModel(model: string): Promise<Configs>
  RenameConfig(index: number, name: string): Promise<Configs>
  DeleteConfig(index: number): Promise<Configs>
  ListChats(): Promise<ChatSummary[]>
  CreateChat(configId: string, model: string, title: string, personaId: string): Promise<ChatTask>
  ListPersonas(): Promise<Persona[]>
  GetChat(id: string): Promise<ChatTask>
  RenameChat(id: string, title: string): Promise<ChatTask>
  DeleteChat(id: string): Promise<void>
  SetChatPersona(id: string, personaId: string): Promise<ChatTask>
  UpdateChatModel(id: string, configId: string, model: string): Promise<ChatTask>
  SetChatReasoningEffort(id: string, effort: string): Promise<ChatTask>
  ListToolSets(): Promise<ToolSetDescriptor[]>
  SetChatToolConfig(id: string, raw: string): Promise<ChatTask>
  SetChatBranch(id: string, parentId: string, choice: number): Promise<ChatTask>
  SendChatMessage(id: string, text: string, reasoningEffort: string): Promise<void>
	SubmitUserAnswer(chatId: string, requestId: string, answer: string): Promise<void>
	SubmitToolApproval(chatId: string, requestId: string, choice: ToolApprovalChoice): Promise<ChatToolConfig | null>
  EditChatMessage(id: string, nodeId: string, text: string, reasoningEffort: string): Promise<void>
  RegenerateChatMessage(id: string, nodeId: string, reasoningEffort: string): Promise<void>
}

export interface ClientExtension {
  type: string
  name: string
  enabled: boolean
  adapterId?: string
}

export interface ExtensionParameter {
  key: string
  label: string
  kind: 'text' | 'secret' | 'integer' | 'boolean'
  required: boolean
  default?: string
  placeholder?: string
  help?: string
  pattern?: string
  minimum?: number
  maximum?: number
  editableWhileRunning: boolean
  enabledWhen?: string
}

export interface ExtensionInformation {
  key: string
  label: string
  inList: boolean
}

export interface ExtensionDescriptor {
  type: string
  implementation: string
  name: string
  description?: string
  needsModel: boolean
  parameters: ExtensionParameter[]
  information: ExtensionInformation[]
}

export interface ExtensionOption {
  id: string
  type: string
  name: string
  clientId?: string
  clientName?: string
  enabled: boolean
  running: boolean
  values: Record<string, string>
  lastError?: string
}

export interface ExtensionClient {
  id: string
  name: string
  extensions: ClientExtension[]
}

export interface ExtensionChatConfig {
  configId: string
  model: string
  personaId: string
  reasoningEffort: string
  toolConfig: ChatToolConfig | null
}

export interface QQContextInfo {
  id: string
  key: string
  kind: 'person' | 'private' | 'group' | 'agent'
  botId: string
  userId?: string
  groupId?: string
  workspace: Workspace
  updatedAt: string
}

export function GetClientExtensionChatConfig(clientId: string, extensionId: string): Promise<ExtensionChatConfig> {
  return bridge().then(c => c.GetClientExtensionChatConfig(clientId, extensionId))
}
export function UpdateClientExtensionChatConfig(clientId: string, extensionId: string, config: ExtensionChatConfig): Promise<ExtensionChatConfig> {
  return bridge().then(c => c.UpdateClientExtensionChatConfig(clientId, extensionId, config))
}
export function ListQQContexts(clientId: string, extensionId: string): Promise<QQContextInfo[]> {
  return bridge().then(c => c.ListQQContexts(clientId, extensionId))
}
export function ClearQQContext(clientId: string, extensionId: string, contextId: string): Promise<void> {
  return bridge().then(c => c.ClearQQContext(clientId, extensionId, contextId))
}
export function DeleteQQContext(clientId: string, extensionId: string, contextId: string): Promise<void> {
  return bridge().then(c => c.DeleteQQContext(clientId, extensionId, contextId))
}

export interface ChatSummary {
  id: string
  title: string
  configId: string
  model: string
  workspaceId?: string
  updatedAt: string
}

export interface ChatTreeNode {
  id: string
  choice: number
  next: ChatTreeNode[] | null
  child: StoredChatMessage[] | null
  usage?: TokenUsage
  reasoningEffort?: string
}

export interface ChatTree {
  root: { choice: number; next: ChatTreeNode[] | null; name: string; personaId?: string } | null
}

// 人设：用于选择器和详情展示，后端将它作为 system 消息发送。
export interface Persona {
  id: string
  name: string
  description?: string
  system: string
  personality?: string[]
  style?: string
  scenario?: string
}

export interface StoredChatMessage {
  role: string
  content: string
  reasoning?: string
  tool_calls?: { id: string; name: string; arguments: string; display_name?: string; display_type?: string; display_content?: string; approval?: ToolApproval }[]
  tool_call_id?: string
  tool_error?: string
}

export interface ChatTask extends ChatSummary {
  tree: ChatTree
  createdAt: string
  // 每个对话各自的思考强度，由后端持久化在会话文件里
  reasoningEffort?: string
  toolConfig?: ChatToolConfig
  memoryLibraryIds?: string[]
}

export interface MemoryLibrary {
  id: string
  name: string
  description: string
  date: string
  scope: 'global' | 'workspace'
  workspaceId?: string
}

export interface MemoryEntry {
  name?: string
  data: string
  date: string
  description: string[] | null
}

export interface MemoryEntryInput {
  name: string
  data: string
  date?: string
  tags: string[]
}

export interface MemoryLibraryDetails extends MemoryLibrary {
  data: Record<string, MemoryEntry>
}

export interface ChatMemorySettings {
  libraries: MemoryLibrary[]
  enabledIds: string[]
}

export type MemoryLibraryAction =
  | { kind: 'create'; name: string; scope: MemoryLibrary['scope'] }
  | { kind: 'rename'; id: string; name: string }
  | { kind: 'delete'; id: string }
  | { kind: 'scope'; id: string; scope: MemoryLibrary['scope'] }

export interface Workspace {
  id: string
  name: string
  path: string
}

export type PermissionLevel = 1 | 2 | 3 | 4

export interface ToolParameterSchema {
  type?: string | string[]
  title?: string
  description?: string
  properties?: Record<string, ToolParameterSchema | boolean>
  required?: string[]
  items?: ToolParameterSchema | boolean | (ToolParameterSchema | boolean)[]
  additionalProperties?: ToolParameterSchema | boolean
  enum?: unknown[]
  default?: unknown
  const?: unknown
  examples?: unknown[]
  [key: string]: unknown
}

export interface ToolDescriptor {
  id: string
  name: string
  description: string
  parameters: ToolParameterSchema | null
  displayType?: string
  displayContent?: string
}

export interface ToolKitDescriptor {
  id: string
  name: string
  description: string
  defaultEnabled: boolean
  tools: ToolDescriptor[]
}

export interface ToolSetDescriptor {
  id: string
  name: string
  description: string
  defaultEnabled: boolean
  kits: ToolKitDescriptor[]
}

// 对话级覆盖：只写显式设置的 ToolKit，未写的项继承全局默认
export interface ChatToolConfig {
  permission?: PermissionLevel
  strictMode?: boolean
  allowedTools?: Record<string, { scope: 'workspace' | 'tool'; workspaceId?: string }>
  sets?: Record<string, { kits?: Record<string, boolean> }>
}

export type ToolApprovalChoice = 'reject' | 'once' | 'workspace' | 'tool' | 'elevate'

export interface ToolApproval {
  callId: string
  toolId: string
  name: string
  arguments: string
  reason: string
  workspaceId?: string
  workspaceName?: string
  workspaceAllowed: boolean
  higherPermission?: PermissionLevel
  decision?: ToolApprovalChoice | 'canceled'
}

export interface ModelProbeResult {
  models: Model[]
  serverAddress: string
}

// window.go 由 Wails 运行时注入，时机可能晚于页面脚本，短暂轮询等待。
// 后端统一位于 core/Backend，Wails 绑定路径为 go.Backend.Core。
async function bridge(): Promise<CoreBindings> {
  for (let i = 0; i < 50; i++) {
    const w = window as unknown as {
      go?: {
        Backend?: { Core?: CoreBindings }
      }
    }
    const bindings = w.go?.Backend?.Core
    if (bindings) return bindings
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error('Wails 绑定尚未就绪')
}

// QQ 适配器管理。
export function ListClientExtensions(clientId: string): Promise<ExtensionOption[]> { return bridge().then((c) => c.ListClientExtensions(clientId)) }
export function CreateClientExtension(clientId: string, type: string, parameters: Record<string, string>): Promise<ExtensionOption> { return bridge().then((c) => c.CreateClientExtension(clientId, type, parameters)) }
export function GetClientExtensionParameters(clientId: string, id: string): Promise<Record<string, string>> { return bridge().then((c) => c.GetClientExtensionParameters(clientId, id)) }
export function UpdateClientExtensionParameters(clientId: string, id: string, parameters: Record<string, string>): Promise<void> { return bridge().then((c) => c.UpdateClientExtensionParameters(clientId, id, parameters)) }
export function DeleteClientExtension(clientId: string, id: string): Promise<void> { return bridge().then((c) => c.DeleteClientExtension(clientId, id)) }
export function ListExtensionTypes(): Promise<ExtensionDescriptor[]> { return bridge().then((c) => c.ListExtensionTypes()) }
export function SetClientExtensionEnabled(clientId: string, extensionId: string, enabled: boolean): Promise<ExtensionClient> { return bridge().then((c) => c.SetClientExtensionEnabled(clientId, extensionId, enabled)) }
export function ListExtensionClients(): Promise<ExtensionClient[]> { return bridge().then((c) => c.ListExtensionClients()) }
export function GetExtensionClient(id: string): Promise<ExtensionClient> { return bridge().then((c) => c.GetExtensionClient(id)) }
export function CreateExtensionClient(name: string, id: string): Promise<ExtensionClient> { return bridge().then((c) => c.CreateExtensionClient(name, id)) }
export function RenameExtensionClient(id: string, name: string): Promise<ExtensionClient> { return bridge().then((c) => c.RenameExtensionClient(id, name)) }
export function DeleteExtensionClient(id: string): Promise<void> { return bridge().then((c) => c.DeleteExtensionClient(id)) }
export function SetExtensionClientQQ(id: string, adapterId: string, name: string, enabled: boolean): Promise<ExtensionClient> { return bridge().then((c) => c.SetExtensionClientQQ(id, adapterId, name, enabled)) }
export function RemoveExtensionClientQQ(id: string, adapterId: string): Promise<ExtensionClient> { return bridge().then((c) => c.RemoveExtensionClientQQ(id, adapterId)) }

// 返回选中的文件夹路径；用户取消时返回空字符串。
export function SelectDirectory(defaultDirectory = ''): Promise<string> {
  return bridge().then((c) => c.SelectDirectory(defaultDirectory))
}

export function ListWorkspaces(): Promise<Workspace[]> {
  return bridge().then((c) => c.ListWorkspaces())
}

export function CreateWorkspace(name: string, path: string): Promise<Workspace> {
  return bridge().then((c) => c.CreateWorkspace(name, path))
}

export function RenameWorkspace(id: string, name: string): Promise<Workspace> {
  return bridge().then((c) => c.RenameWorkspace(id, name))
}

export function DeleteWorkspace(id: string): Promise<void> {
  return bridge().then((c) => c.DeleteWorkspace(id))
}

export function DeleteWorkspaceAndChats(id: string): Promise<void> {
  return bridge().then((c) => c.DeleteWorkspaceAndChats(id))
}

export function SetChatWorkspace(id: string, workspaceId: string): Promise<ChatTask> {
  return bridge().then((c) => c.SetChatWorkspace(id, workspaceId))
}

export function LoadConfigs(): Promise<Configs> {
  return bridge().then((c) => c.LoadConfigs())
}

export function GetChatModelContextWindow(id: string): Promise<number> {
  return bridge().then((c) => c.GetChatModelContextWindow(id))
}

export function ProbeModels(serverAddress: string, apiKey: string): Promise<ModelProbeResult> {
  return bridge().then((c) => c.ProbeModels(serverAddress, apiKey))
}

export function AddModelConfig(
  name: string,
  serverAddress: string,
  protocol: Config['Protocol'],
  apiKey: string,
  models: Model[],
): Promise<Configs> {
  return bridge().then((c) => c.AddModelConfig(name, serverAddress, protocol, apiKey, models))
}

export function RefreshModelConfig(index: number, serverAddress: string, models: Model[]): Promise<Configs> {
  return bridge().then((c) => c.RefreshModelConfig(index, serverAddress, models))
}

export function SelectConfig(index: number): Promise<Configs> {
  return bridge().then((c) => c.SelectConfig(index))
}

export function SelectModel(model: string): Promise<Configs> {
  return bridge().then((c) => c.SelectModel(model))
}

export function RenameConfig(index: number, name: string): Promise<Configs> {
  return bridge().then((c) => c.RenameConfig(index, name))
}

export function DeleteConfig(index: number): Promise<Configs> {
  return bridge().then((c) => c.DeleteConfig(index))
}

export function ListChats(): Promise<ChatSummary[]> { return bridge().then((c) => c.ListChats()) }
export function CreateChat(configId = '', model = '', title = '', personaId = ''): Promise<ChatTask> {
  return bridge().then((c) => c.CreateChat(configId, model, title, personaId))
}
export function ListPersonas(): Promise<Persona[]> {
  return bridge().then((c) => c.ListPersonas())
}
export function GetChat(id: string): Promise<ChatTask> { return bridge().then((c) => c.GetChat(id)) }
export function GetChatMemorySettings(id = ''): Promise<ChatMemorySettings> {
  return bridge().then((c) => c.GetChatMemorySettings(id))
}
export function GetWorkspaceMemorySettings(workspaceId = ''): Promise<ChatMemorySettings> {
  return bridge().then((c) => c.GetWorkspaceMemorySettings(workspaceId))
}
export function GetMemoryLibraryDetails(chatId: string, workspaceId: string, id: string): Promise<MemoryLibraryDetails> {
  return bridge().then((c) => c.GetMemoryLibraryDetails(chatId, workspaceId, id))
}
export function CreateMemoryEntry(chatId: string, workspaceId: string, libraryId: string, input: MemoryEntryInput): Promise<MemoryLibraryDetails> {
  return bridge().then((c) => c.CreateMemoryEntry(chatId, workspaceId, libraryId, input))
}
export function UpdateMemoryEntry(chatId: string, workspaceId: string, libraryId: string, memoryId: string, input: MemoryEntryInput): Promise<MemoryLibraryDetails> {
  return bridge().then((c) => c.UpdateMemoryEntry(chatId, workspaceId, libraryId, memoryId, input))
}
export function DeleteMemoryEntry(chatId: string, workspaceId: string, libraryId: string, memoryId: string): Promise<MemoryLibraryDetails> {
  return bridge().then((c) => c.DeleteMemoryEntry(chatId, workspaceId, libraryId, memoryId))
}
export function SetChatMemoryLibraries(id: string, ids: string[]): Promise<ChatTask> {
  return bridge().then((c) => c.SetChatMemoryLibraries(id, ids))
}
export function CreateMemoryLibrary(chatId: string, workspaceId: string, name: string, scope: MemoryLibrary['scope']): Promise<MemoryLibrary> {
  return bridge().then((c) => c.CreateMemoryLibrary(chatId, workspaceId, name, scope))
}
export function RenameMemoryLibrary(chatId: string, workspaceId: string, id: string, name: string): Promise<void> {
  return bridge().then((c) => c.RenameMemoryLibrary(chatId, workspaceId, id, name))
}
export function DeleteMemoryLibrary(chatId: string, workspaceId: string, id: string): Promise<void> {
  return bridge().then((c) => c.DeleteMemoryLibrary(chatId, workspaceId, id))
}
export function SetMemoryLibraryScope(chatId: string, workspaceId: string, id: string, scope: MemoryLibrary['scope']): Promise<void> {
  return bridge().then((c) => c.SetMemoryLibraryScope(chatId, workspaceId, id, scope))
}
export function RenameChat(id: string, title: string): Promise<ChatTask> {
  return bridge().then((c) => c.RenameChat(id, title))
}
export function DeleteChat(id: string): Promise<void> { return bridge().then((c) => c.DeleteChat(id)) }
export function SetChatPersona(id: string, personaId: string): Promise<ChatTask> {
  return bridge().then((c) => c.SetChatPersona(id, personaId))
}
export function UpdateChatModel(id: string, configId: string, model: string): Promise<ChatTask> {
  return bridge().then((c) => c.UpdateChatModel(id, configId, model))
}
export function SetChatReasoningEffort(id: string, effort: string): Promise<ChatTask> {
  return bridge().then((c) => c.SetChatReasoningEffort(id, effort))
}
export function ListToolSets(): Promise<ToolSetDescriptor[]> {
  return bridge().then((c) => c.ListToolSets())
}
export function SetChatToolConfig(id: string, raw: string): Promise<ChatTask> {
  return bridge().then((c) => c.SetChatToolConfig(id, raw))
}
export function SetChatBranch(id: string, parentId: string, choice: number): Promise<ChatTask> {
  return bridge().then((c) => c.SetChatBranch(id, parentId, choice))
}
export function SendChatMessage(id: string, text: string, reasoningEffort: string): Promise<void> {
  return bridge().then((c) => c.SendChatMessage(id, text, reasoningEffort))
}
export function EditChatMessage(id: string, nodeId: string, text: string, reasoningEffort: string): Promise<void> {
  return bridge().then((c) => c.EditChatMessage(id, nodeId, text, reasoningEffort))
}
export function RegenerateChatMessage(id: string, nodeId: string, reasoningEffort: string): Promise<void> {
  return bridge().then((c) => c.RegenerateChatMessage(id, nodeId, reasoningEffort))
}

// "chat:delta" 事件载荷:文本增量与思考增量分开携带(可同时)。
export interface ChatDeltaEvent {
  chatId?: string
  nodeId?: string
  content?: string
  reasoning?: string
  usage?: TokenUsage
}

export interface TokenUsage {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  completion_tokens_details?: { reasoning_tokens?: number }
}

// "chat:tool" 事件载荷:工具调用的开始/结束。
export interface ChatToolEvent {
  chatId?: string
  nodeId?: string
  toolId?: string
  callId?: string
  name: string
  event: 'start' | 'update' | 'done' | 'fail'
  error?: string
  displayType?: string
  displayContent?: string
  content?: string
  requestId?: string
  answer?: string
  approval?: string | ToolApproval
}

export async function SubmitToolApproval(chatId: string, requestId: string, choice: ToolApprovalChoice): Promise<ChatToolConfig | null> {
  return (await bridge()).SubmitToolApproval(chatId, requestId, choice)
}

export async function SubmitUserAnswer(chatId: string, requestId: string, answer: string): Promise<void> {
  return (await bridge()).SubmitUserAnswer(chatId, requestId, answer)
}

function parseEvent<T>(data: unknown): T {
  // 事件载荷的契约是 JSON 字符串;兼容三种输入:已是对象、JSON 字符串、纯文本。
  if (data && typeof data === 'object') return data as T
  const text = String(data)
  try {
    const obj = JSON.parse(text)
    if (obj && typeof obj === 'object') return obj as T
  } catch {
    /* 非 JSON 的纯文本按 content 增量处理,兼容旧契约 */
  }
  return { content: text } as T
}

// 订阅流式增量事件 "chat:delta",返回取消订阅函数。
// runtime 注入时机可能晚于组件挂载,短暂轮询等待。
export async function onChatDelta(
  handler: (delta: ChatDeltaEvent) => void,
): Promise<() => void> {
  for (let i = 0; i < 50; i++) {
    const rt = (
      window as unknown as {
        runtime?: {
          EventsOn(name: string, cb: (data: unknown) => void): void
          EventsOff(...names: string[]): void
        }
      }
    ).runtime
    if (rt) {
      rt.EventsOn('chat:delta', (data) => handler(parseEvent<ChatDeltaEvent>(data)))
      return () => rt.EventsOff('chat:delta')
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  return () => {}
}

// 订阅工具调用事件 "chat:tool",返回取消订阅函数。
export async function onChatTool(
  handler: (tool: ChatToolEvent) => void,
): Promise<() => void> {
  for (let i = 0; i < 50; i++) {
    const rt = (
      window as unknown as {
        runtime?: {
          EventsOn(name: string, cb: (data: unknown) => void): void
          EventsOff(...names: string[]): void
        }
      }
    ).runtime
    if (rt) {
      rt.EventsOn('chat:tool', (data) => handler(parseEvent<ChatToolEvent>(data)))
      return () => rt.EventsOff('chat:tool')
    }
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  return () => {}
}
