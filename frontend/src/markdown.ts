import { marked, Renderer } from 'marked'
import DOMPurify from 'dompurify'

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

const renderer = new Renderer()

// 代码块带语言标签 + 复制按钮。按钮不带事件，点击由 ChatPage 做委托，
// 从旁边的 <pre><code> 读原文，避免把整段代码塞进 HTML 属性里。
renderer.code = ({ text, lang }) => {
  const language = (lang ?? '').trim().split(/\s+/)[0] || 'text'
  return `<div class="code-block">
  <div class="code-head">
    <span class="code-lang">${escapeHtml(language)}</span>
    <button type="button" class="code-copy">复制</button>
  </div>
  <pre><code>${escapeHtml(text)}</code></pre>
</div>`
}

marked.setOptions({ gfm: true, breaks: true, renderer })

// 流式输出常停在半截语法上（"##"、"**粗体"、未闭合代码块）。
// 这里临时补齐定界符，避免界面露出裸标记；补全只用于渲染，不改消息原文。
export function balanceMarkdown(text: string): string {
  let out = text

  const lines = out.split('\n')
  const isFence = (line: string) => /^\s{0,3}(```|~~~)/.test(line)

  // 1. 奇数个围栏 = 代码块还没写完
  if (lines.filter(isFence).length % 2 === 1) out += '\n```'

  // 2. 行内定界符（排除围栏行，``` 会被误算成 3 个反引号）
  const body = lines.filter((line) => !isFence(line)).join('\n')
  const count = (source: string, token: string) => source.split(token).length - 1
  if (count(body, '`') % 2 === 1) out += '`'
  if (count(body, '**') % 2 === 1) out += '**'
  if (count(body, '~~') % 2 === 1) out += '~~'

  // 3. 末尾只有标记前缀、正文还没到（"##"、"- "、"1. "）→ 暂时丢掉这一截
  out = out.replace(/(^|\n)(?:#{1,6}|[-*+]\s+|>\s+|\d+\.\s+)\s*$/, '$1')

  return out
}

function parse(text: string): string {
  return DOMPurify.sanitize(marked.parse(balanceMarkdown(text), { async: false }) as string)
}

// ===== 已完成的消息：缓存 HTML，滚动、切分支都不再重新解析 =====
const doneCache = new Map<string, { text: string; html: string }>()

export function renderDone(key: string, text: string): string {
  const hit = doneCache.get(key)
  if (hit && hit.text === text) return hit.html
  const html = parse(text)
  if (doneCache.size > 500) doneCache.clear()
  doneCache.set(key, { text, html })
  return html
}

// ===== 流式消息：换行数变化或超过 100ms 才重解析，其余 delta 复用上一帧 =====
let streamHtml = ''
let streamLineCount = -1
let streamAt = 0

export function renderStreaming(text: string): string {
  const lineCount = text.split('\n').length
  const now = performance.now()
  if (text === '' || lineCount !== streamLineCount || now - streamAt > 100) {
    streamHtml = parse(text)
    streamLineCount = lineCount
    streamAt = now
  }
  return streamHtml
}

export function resetStreaming() {
  streamHtml = ''
  streamLineCount = -1
  streamAt = 0
}
