import { loginTestProfile } from './profile_bridge.mjs'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'

const frontend = path.resolve('frontend')
const require = createRequire(path.join(frontend, 'package.json'))
const { transformWithOxc } = await import(pathToFileURL(require.resolve('vite')).href)
const { parse, compileScript } = await import(pathToFileURL(require.resolve('@vue/compiler-sfc')).href)
const { createSSRApp, createRenderer, reactive, h, nextTick } = await import(pathToFileURL(require.resolve('vue')).href)
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

// Approval events preserve the tool's normal presentation and restore an inactive audit record from history.
const approval = { callId: 'blocked', toolId: 'WriteFile', name: '覆盖写入文件', arguments: '{"path":["file.txt"],"content":"原始正文"}',
  reason: '当前权限不允许写入', workspaceId: 'workspace', workspaceName: '项目', workspaceAllowed: true, higherPermission: 2 }
chat.messages = [{ role: 'assistant', content: '' }]
chat.applyTool({ chatId: 'chat', callId: 'approval-call', toolId: 'WriteFile', name: '覆盖写入文件', event: 'start', displayType: 'info', displayContent: 'arguments.content', content: approval.arguments })
chat.applyTool({ chatId: 'chat', callId: 'approval-call', event: 'update', requestId: 'approval-request', approval: JSON.stringify(approval) })
item = chat.messages[0].process[0]
assert.deepEqual(item.approval, approval)
html = await render(item)
assert.match(html, /等待审批/)
assert.match(html, /允许本次/)
assert.match(html, /拒绝/)
assert.match(html, /原始正文/)
assert.match(html, /file\.txt/)
chat.applyTool({ chatId: 'chat', callId: 'approval-call', event: 'done', approval: { ...approval, decision: 'workspace' } })
html = await render(item)
assert.doesNotMatch(html, /更多允许选项/)
assert.match(html, /已允许在此工作区内使用这个工具/)
assert.equal(item.requestId, undefined)

const historyCall = chat.session.tree.root.next[0].next[0].child[4].tool_calls[0]
historyCall.approval = { ...approval, decision: 'once' }
chat.rebuildMessages()
assert.deepEqual(chat.messages[1].process[2].approval, historyCall.approval)

// Exercise actual button handlers, menus, duplicate-submit protection, errors and approval/config replies.
const root = element('root'), body = element('body')
function element(type) { return { type, props: {}, children: [], parent: null, text: '', getBoundingClientRect: () => ({ height: 125 }), contains: node => false } }
function detach(node) { if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1) }
function find(node, predicate) { if (predicate(node)) return node; for (const child of node.children) { const match = find(child, predicate); if (match) return match } }
function all(node, predicate) { return [...(predicate(node) ? [node] : []), ...node.children.flatMap(child => all(child, predicate))] }
const nodeText = node => node.text + node.children.map(nodeText).join('')
const renderer = createRenderer({
  createElement: element, createText: text => ({ ...element('#text'), text }), createComment: text => element('#comment'),
  setText: (node, text) => { node.text = text }, setElementText: (node, text) => { node.text = text; node.children = [] },
  patchProp: (node, key, previous, value) => { node.props[key] = value },
  insert(node, parent, anchor = null) { detach(node); node.parent = parent; parent.children.splice(anchor ? parent.children.indexOf(anchor) : parent.children.length, 0, node) },
  remove: detach, parentNode: node => node.parent, nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
  querySelector: () => body,
})
const handlers = new Map()
globalThis.document = { addEventListener: (name, handler) => handlers.set(name, handler), removeEventListener: name => handlers.delete(name) }
let finishApproval, failApproval, approvalCalls = []
globalThis.window = { innerWidth: 420, innerHeight: 550, addEventListener() {}, removeEventListener() {}, go: { Backend: { Core: {
  SubmitToolApproval: (...args) => { approvalCalls.push(args); return new Promise((resolve, reject) => { finishApproval = resolve; failApproval = reject }) },
} } } }
await loginTestProfile(apiURL)
const previews = []; window.go.Backend.Core.GetChatImage = async (chat, id) => {previews.push([chat,id]);return 'data:image/png;base64,dGVzdA=='}
const state = reactive({ item: { kind: 'tool', callId: 'ui-call', status: 'run', requestId: 'ui-request', approval, displayType: 'info', displayContent: 'arguments.content', content: approval.arguments }, chatId: 'chat' })
const accepted = []
const app = renderer.createApp({ render: () => h(ToolCallDetails, { ...state, onApproved: value => accepted.push(value) }) })
app.mount(root)
const flush = async () => { await new Promise(resolve => setImmediate(resolve)); await nextTick() }
const buttons = () => all(root, node => node.type === 'button')
const button = text => buttons().find(node => nodeText(node).trim() === text)
const menuButton = () => buttons().find(node => node.props['aria-label'] === '更多允许选项')
const toggleMenu = async () => { await menuButton().props.onClick({ currentTarget: { getBoundingClientRect: () => ({ right: 410, bottom: 480, top: 450 }) } }); await flush() }
await flush()
await toggleMenu()
let menu = find(body, node => node.props.role === 'menu')
assert.ok(menu)
assert.match(nodeText(menu), /允许工作区内使用此工具/)
assert.match(nodeText(menu), /允许此工具/)
assert.match(nodeText(menu), /切换为更高权限.*自动编辑/)
assert.equal(menu.props.style.left, '158px')
assert.equal(menu.props.style.top, '320px')
handlers.get('pointerdown')({ target: {} })
await flush()
assert.equal(find(body, node => node.props.role === 'menu'), undefined)
let submitted = button('允许本次').props.onClick()
await flush()
assert.deepEqual(approvalCalls, [['chat', 'ui-request', 'once']])
await button('拒绝').props.onClick()
assert.equal(approvalCalls.length, 1, 'approval writes must not be duplicated')
failApproval(new Error('审批保存失败'))
await submitted
await flush()
assert.match(nodeText(root), /审批保存失败/)
assert.ok(button('允许本次'), 'a failure must keep the pending approval usable')
submitted = button('允许本次').props.onClick()
await flush()
const approvedConfig = { permission: 1, strictMode: true, allowedTools: { WriteFile: { scope: 'workspace', workspaceId: 'workspace' } } }
finishApproval(approvedConfig)
await submitted
await flush()
assert.deepEqual(accepted[0], { chatId: 'chat', config: approvedConfig })
assert.equal(button('允许本次'), undefined)
assert.match(nodeText(root), /已允许本次/)

state.item = { ...state.item, callId: 'external-call', requestId: 'external-request', approval: { ...approval, workspaceAllowed: false, higherPermission: 4 } }
await flush()
await toggleMenu()
menu = find(body, node => node.props.role === 'menu')
assert.doesNotMatch(nodeText(menu), /允许工作区内使用此工具/)
assert.match(nodeText(menu), /完全访问/)
state.chatId = 'different-chat'
state.item = { ...state.item, callId: 'changed-call', requestId: undefined, status: 'fail', approval: { ...approval, decision: 'canceled' } }
await flush()
assert.equal(find(body, node => node.props.role === 'menu'), undefined, 'a previous approval menu must close after context changes')
const img = {id:'attachment-1',name:'截图.png',mime:'image/png'}
chat.messages = [{role:'assistant',content:''}]
chat.applyTool({event:'image',chatId:'chat',name:'读取图片',image:img})
chat.applyTool({event:'image',chatId:'chat',name:'读取图片',image:img})
assert.equal(chat.messages[0].process.length,1,'one receipt for one direct image')
state.chatId='chat';state.item=chat.messages[0].process[0];await flush()
assert.match(nodeText(root),/已读取图片.*截图.png/);assert.equal(previews.length,0)
const imageDetails=find(root,n=>n.type==='details');imageDetails.props.onToggle({target:{open:true}});await flush()
assert.deepEqual(previews,[['chat',img.id]]);assert.equal(find(root,n=>n.type==='img').props.src,'data:image/png;base64,dGVzdA==')
imageDetails.props.onToggle({target:{open:false}});await flush();assert.equal(find(root,n=>n.type==='img'),undefined)
state.item={kind:'tool',displayType:'image',status:'done',image:img,imageSent:false};await flush();assert.doesNotMatch(nodeText(root),/已读取图片/)
chat.session.tree.root.next[0].next[0].inputImages=[img];chat.rebuildMessages();assert.equal(chat.messages[1].process[0].imageSent,true)
app.unmount()
assert.equal(handlers.size, 0)

chat.session.toolConfig = { permission: 1, strictMode: false }
chat.applyApprovalConfig({ chatId: 'other', config: approvedConfig })
assert.equal(chat.strictMode, false)
chat.applyApprovalConfig({ chatId: 'chat', config: approvedConfig })
assert.equal(chat.strictMode, true)
assert.deepEqual(chat.session.toolConfig.allowedTools, approvedConfig.allowedTools)
console.log('Tool display checks passed: live question events, history restoration, expansion, form, and read/write/search content.')
