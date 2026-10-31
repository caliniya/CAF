// 与 Go 侧 core.Configs / core.Config 对应的 JSON 结构。
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
}

interface CoreBindings {
  LoadConfigs(): Promise<Configs>
  GetModelContextWindow(): Promise<number>
  GetChatModelContextWindow(id: string): Promise<number>
  ProbeModels(serverAddress: string, apiKey: string): Promise<ModelProbeResult>
  AddModelConfig(name: string, serverAddress: string, protocol: Config['Protocol'], apiKey: string, models: Model[]): Promise<Configs>
  RefreshModelConfig(index: number, serverAddress: string, models: Model[]): Promise<Configs>
  SelectConfig(index: number): Promise<Configs>
  SelectModel(model: string): Promise<Configs>
  RenameConfig(index: number, name: string): Promise<Configs>
  StartLogin(): Promise<number>
  ListChats(): Promise<ChatSummary[]>
  CreateChat(configId: string, model: string, title: string): Promise<ChatTask>
  GetChat(id: string): Promise<ChatTask>
  RenameChat(id: string, title: string): Promise<ChatTask>
  DeleteChat(id: string): Promise<void>
  UpdateChatModel(id: string, configId: string, model: string): Promise<ChatTask>
  SetChatBranch(id: string, parentId: string, choice: number): Promise<ChatTask>
  SendChatMessage(id: string, text: string, reasoningEffort: string): Promise<void>
  RegenerateChatMessage(id: string, nodeId: string, reasoningEffort: string): Promise<void>
}

export interface ChatSummary {
  id: string
  title: string
  configId: string
  model: string
  updatedAt: string
}

export interface ChatTreeNode {
  id: string
  choice: number
  next: ChatTreeNode[]
  child: StoredChatMessage[]
}

export interface ChatTree {
  root: { choice: number; next: ChatTreeNode[]; name: string }
}

export interface StoredChatMessage {
  Role: string
  Content: string
  Reasoning?: string
  ToolCalls?: { ID: string; Name: string; Arguments: string }[]
  ToolCallID?: string
}

export interface ChatTask extends ChatSummary {
  tree: ChatTree
  createdAt: string
}

export interface ModelProbeResult {
  models: Model[]
  serverAddress: string
}

// window.go 由 Wails 运行时注入，时机可能晚于页面脚本，短暂轮询等待。
async function bridge(): Promise<CoreBindings> {
  for (let i = 0; i < 50; i++) {
    const w = window as unknown as {
      go?: { core?: { Core?: CoreBindings } }
    }
    if (w.go?.core?.Core) return w.go.core.Core
    await new Promise((resolve) => setTimeout(resolve, 100))
  }
  throw new Error('Wails 绑定尚未就绪')
}

export function LoadConfigs(): Promise<Configs> {
  return bridge().then((c) => c.LoadConfigs())
}

export function GetModelContextWindow(): Promise<number> {
  return bridge().then((c) => c.GetModelContextWindow())
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

export function ListChats(): Promise<ChatSummary[]> { return bridge().then((c) => c.ListChats()) }
export function CreateChat(configId = '', model = '', title = ''): Promise<ChatTask> {
  return bridge().then((c) => c.CreateChat(configId, model, title))
}
export function GetChat(id: string): Promise<ChatTask> { return bridge().then((c) => c.GetChat(id)) }
export function RenameChat(id: string, title: string): Promise<ChatTask> {
  return bridge().then((c) => c.RenameChat(id, title))
}
export function DeleteChat(id: string): Promise<void> { return bridge().then((c) => c.DeleteChat(id)) }
export function UpdateChatModel(id: string, configId: string, model: string): Promise<ChatTask> {
  return bridge().then((c) => c.UpdateChatModel(id, configId, model))
}
export function SetChatBranch(id: string, parentId: string, choice: number): Promise<ChatTask> {
  return bridge().then((c) => c.SetChatBranch(id, parentId, choice))
}
export function SendChatMessage(id: string, text: string, reasoningEffort: string): Promise<void> {
  return bridge().then((c) => c.SendChatMessage(id, text, reasoningEffort))
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
}

// "chat:tool" 事件载荷:工具调用的开始/结束。
export interface ChatToolEvent {
  chatId?: string
  nodeId?: string
  name: string
  event: 'start' | 'done'
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
