// 与 Go 侧 core.Configs / core.Config 对应的 JSON 结构。
export interface Config {
  Name: string
  ServerAddress: string
  APIKey: string
  Model: string[]
  // 该配置自己记住的启用模型；每个提供商独立记忆，切换提供商自动恢复。
  EnabledModel: string
}

export interface Configs {
  config: Config[] | null
  EnablesConfig: Config | null
}

interface CoreBindings {
  LoadConfigs(): Promise<Configs>
  SelectConfig(index: number): Promise<Configs>
  SelectModel(model: string): Promise<Configs>
  RenameConfig(index: number, name: string): Promise<Configs>
  StartLogin(): Promise<number>
  /**
   * 【后端待实现】发起流式对话：
   *   Go 侧签名：func (c *Core) SendMessage(history []netclient.ChatMessage) error
   *   history 元素形如 {"role":"user"|"assistant","content":"..."}；
   *   实现方需边流式接收边用 runtime.EventsEmit 推送事件，函数返回（或返回
   *   error）即代表对话结束，前端据此收尾。事件契约：
   *     "chat:delta"  载荷为 JSON 字符串 {"content"?: string, "reasoning"?: string}
   *     "chat:tool"   载荷为 JSON 字符串 {"name": string, "event": "start"|"done"}
   */
  SendMessage(history: ChatTurn[]): Promise<void>
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

export function SelectConfig(index: number): Promise<Configs> {
  return bridge().then((c) => c.SelectConfig(index))
}

export function SelectModel(model: string): Promise<Configs> {
  return bridge().then((c) => c.SelectModel(model))
}

export function RenameConfig(index: number, name: string): Promise<Configs> {
  return bridge().then((c) => c.RenameConfig(index, name))
}

// 一条对话消息；role 为 "user" 或 "assistant"。
export interface ChatTurn {
  role: string
  content: string
}

export function SendMessage(history: ChatTurn[]): Promise<void> {
  return bridge().then((c) => c.SendMessage(history))
}

// "chat:delta" 事件载荷:文本增量与思考增量分开携带(可同时)。
export interface ChatDeltaEvent {
  content?: string
  reasoning?: string
}

// "chat:tool" 事件载荷:工具调用的开始/结束。
export interface ChatToolEvent {
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
