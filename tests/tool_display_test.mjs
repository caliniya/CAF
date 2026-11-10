import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'

const frontend = path.resolve('frontend')
const require = createRequire(path.join(frontend, 'package.json'))
const { transformWithOxc } = await import(pathToFileURL(require.resolve('vite')).href)
const { parse, compileScript } = await import(pathToFileURL(require.resolve('@vue/compiler-sfc')).href)
const { createSSRApp, h } = await import(pathToFileURL(require.resolve('vue')).href)
const { renderToString } = await import(pathToFileURL(require.resolve('@vue/server-renderer')).href)
const { createPinia, setActivePinia } = await import(pathToFileURL(require.resolve('pinia')).href)
const dataModule = (code) => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const apiCode = (await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/api.ts'), 'utf8'), 'api.ts')).code
const apiURL = dataModule(apiCode)
const configsURL = dataModule('export const useConfigsStore = () => ({})')
function rewrite(code) {
  return code.replace(/from\s+(['"])([^'"]+)\1/g, (whole, quote, specifier) => {
    const url = specifier === '../api' ? apiURL : specifier === './configs' ? configsURL
      : pathToFileURL(require.resolve(specifier)).href
    return `from ${JSON.stringify(url)}`
  })
}
const storeCode = (await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/stores/chat.ts'), 'utf8'), 'chat.ts')).code
const { useChatStore } = await import(dataModule(rewrite(storeCode)))
setActivePinia(createPinia())
const chat = useChatStore()
chat.session = { id: 'chat', tree: { root: { choice: -1, next: [] } } }
chat.messages = [{ role: 'assistant', content: '中间说明' }]
chat.applyTool({ chatId: 'chat', callId: 'call', name: '向用户提问', event: 'start', displayType: 'question', displayContent: 'arguments', content: '{"question":"选择方案？"}' })
chat.applyTool({ chatId: 'other', callId: 'call', name: '向用户提问', event: 'update', requestId: 'wrong' })
let item = chat.messages[0].process[1]
assert.equal(item.requestId, undefined)
chat.applyTool({ chatId: 'chat', callId: 'call', name: '向用户提问', event: 'update', requestId: 'request', content: '{"question":"选择方案？","options":["A","B"]}' })
assert.equal(item.status, 'run')
assert.equal(item.requestId, 'request')

const source = fs.readFileSync(path.join(frontend, 'src/components/ToolCallDetails.vue'), 'utf8')
const { descriptor } = parse(source)
const script = compileScript(descriptor, { id: 'tool-check', inlineTemplate: true })
const componentCode = (await transformWithOxc(script.content, 'ToolCallDetails.ts')).code
const { default: ToolCallDetails } = await import(dataModule(rewrite(componentCode)))
const render = (item) => renderToString(createSSRApp({ render: () => h(ToolCallDetails, { item, chatId: 'chat' }) }))
let html = await render(item)
assert.match(html, /<details[^>]* open/)
assert.match(html, /<form/)
assert.match(html, /选择方案？/)
assert.match(html, /提交回答/)
chat.applyTool({ chatId: 'chat', callId: 'call', name: '向用户提问', event: 'done', answer: '自定义回答' })
assert.equal(item.status, 'done')
assert.equal(item.requestId, undefined)
html = await render(item)
assert.doesNotMatch(html, /<details[^>]* open/)
assert.doesNotMatch(html, /<form/)
assert.match(html, /回答：自定义回答/)
html = await render({ kind: 'tool', name: '读取文件', status: 'done', displayType: 'info', displayContent: 'result', content: '<script>plain text</script>' })
assert.doesNotMatch(html, /<details[^>]* open/)
assert.match(html, /&lt;script&gt;plain text&lt;\/script&gt;/)
html = await render({ kind: 'tool', name: '信息', status: 'run', displayType: 'info', displayContent: 'arguments', content: '完整信息' })
assert.match(html, /<details[^>]* open/)

for (const status of ['run', 'done']) {
  html = await render({ kind: 'tool', name: '覆盖写入文件', status, displayType: 'info', displayContent: 'arguments.content',
    content: JSON.stringify({ path: ['example.txt'], content: '第一行\n<script>原文</script>\n' }) })
  assert.doesNotMatch(html, /<details[^>]* open/)
  assert.match(html, /第一行\n&lt;script&gt;原文&lt;\/script&gt;\n/)
  assert.doesNotMatch(html, /example\.txt/)
}
html = await render({ kind: 'tool', name: '覆盖写入文件', status: 'done', displayType: 'info', displayContent: 'arguments.content',
  content: JSON.stringify({ path: ['example.txt'], content: '' }) })
assert.match(html, /（空内容）/)
html = await render({ kind: 'tool', name: '搜索文件', status: 'done', displayType: 'info', displayContent: 'result',
  content: JSON.stringify({ path: 'workspace', matches: ['workspace/example.txt'], truncated: true }) })
assert.doesNotMatch(html, /<details[^>]* open/)
assert.match(html, /workspace\/example\.txt/)
assert.match(html, /truncated.*true/)

chat.session = { id: 'chat', tree: { root: { choice: 0, next: [{ id: 'user', choice: 0, child: [{ role: 'user', content: '问题' }], next: [{ id: 'reply', choice: -1, next: [], child: [
  { role: 'assistant', tool_calls: [{ id: 'ask', name: 'AskUser', display_name: '向用户提问', arguments: '{"question":"选择方案？"}', display_type: 'question', display_content: 'arguments' }] },
  { role: 'tool', tool_call_id: 'ask', content: '自定义回答' },
  { role: 'assistant', tool_calls: [{ id: 'read', name: 'ReadFile', arguments: '{}', display_type: 'info', display_content: 'result' }] },
  { role: 'tool', tool_call_id: 'read', content: '读取失败', tool_error: '读取失败' },
  { role: 'assistant', tool_calls: [{ id: 'write', name: 'WriteFile', arguments: '{"path":["example.txt"],"content":"写入的原文"}', display_type: 'info', display_content: 'arguments.content' }] },
  { role: 'tool', tool_call_id: 'write', content: '{"bytes_written":15}' },
  { role: 'assistant', tool_calls: [{ id: 'search', name: 'SearchFiles', arguments: '{"pattern":"*.txt"}', display_type: 'info', display_content: 'result' }] },
  { role: 'tool', tool_call_id: 'search', content: '{"matches":["example.txt"],"truncated":false}' },
  { role: 'assistant', content: '完成', reasoning: '最终思考' },
] }] }] } } }
chat.rebuildMessages()
const process = chat.messages[1].process
assert.equal(process[0].answer, '自定义回答')
assert.equal(process[0].displayType, 'question')
assert.equal(process[1].status, 'fail')
assert.equal(process[1].content, '读取失败')
assert.equal(process.filter((item) => item.kind === 'reasoning').length, 1)
assert.equal(chat.messages[1].content, '完成')
html = await render(process[2])
assert.match(html, /写入的原文/)
assert.doesNotMatch(html, /bytes_written/)
assert.doesNotMatch(html, /<details[^>]* open/)
assert.equal(process[3].content, '{"matches":["example.txt"],"truncated":false}')
console.log('Tool display checks passed: live question events, history restoration, expansion, form, and read/write/search content.')
