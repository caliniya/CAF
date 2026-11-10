import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'

const frontend = path.resolve('frontend')
const require = createRequire(path.join(frontend, 'package.json'))
const { transformWithOxc } = await import(pathToFileURL(require.resolve('vite')).href)
const { parse, compileScript } = await import(pathToFileURL(require.resolve('@vue/compiler-sfc')).href)
const { createRenderer, h, nextTick } = await import(pathToFileURL(require.resolve('vue')).href)
const { createPinia, setActivePinia } = await import(pathToFileURL(require.resolve('pinia')).href)
const dataModule = (code) => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const apiURL = dataModule((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/api.ts'), 'utf8'), 'api.ts')).code)
const configsURL = dataModule('export const useConfigsStore = () => ({ loaded: true, configs: [], enables: null })')
const markdownURL = dataModule('export const renderDone = (_, text) => text; export const renderStreaming = (text) => text; export const resetStreaming = () => {}')
const dialogURL = dataModule('export default { render: () => null }')
const imports = { '../api': apiURL, './configs': configsURL, '../stores/configs': configsURL, '../markdown': markdownURL }
function rewrite(code) {
  return code.replace(/from\s+(['"])([^'"]+)\1/g, (whole, quote, specifier) => {
    const url = imports[specifier] ?? (specifier.endsWith('.vue') ? dialogURL : pathToFileURL(require.resolve(specifier)).href)
    return `from ${JSON.stringify(url)}`
  })
}
const storeURL = dataModule(rewrite((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/stores/chat.ts'), 'utf8'), 'chat.ts')).code))
imports['../stores/chat'] = storeURL
const { useChatStore } = await import(storeURL)
const { descriptor } = parse(fs.readFileSync(path.join(frontend, 'src/pages/ChatPage.vue'), 'utf8'))
const script = compileScript(descriptor, { id: 'chat-edit-check', inlineTemplate: true })
const { default: ChatPage } = await import(dataModule(rewrite((await transformWithOxc(script.content, 'ChatPage.ts')).code)))

function detach(node) {
  if (!node.parent) return
  node.parent.children.splice(node.parent.children.indexOf(node), 1)
  node.parent = null
}
function element(type) {
  return {
    type, tagName: type.toUpperCase(), props: {}, children: [], parent: null, value: '',
    addEventListener() {}, removeEventListener() {},
    getRootNode() { return document },
    focus() { document.activeElement = this },
    setSelectionRange(start, end) { this.selectionStart = start; this.selectionEnd = end },
  }
}
function find(node, predicate) {
  if (predicate(node)) return node
  for (const child of node.children ?? []) {
    const found = find(child, predicate)
    if (found) return found
  }
}
const root = element('root')
const body = element('body')
const editInput = () => find(root, (node) => node.props.class === 'msg-edit-input')
globalThis.Document = class {}
globalThis.ShadowRoot = class {}
globalThis.document = Object.assign(new Document(), { activeElement: null, body, querySelector: () => editInput(), addEventListener() {}, removeEventListener() {} })
const listeners = new Map()
let editCalls = 0
let editedArguments
let completeEdit
let rejectEdit
const userNode = (id, content, next = []) => ({ id, choice: next.length ? 0 : -1, child: [{ role: 'user', content }], next })
const replyNode = (id, content, next = []) => ({ id, choice: next.length ? 0 : -1, child: [{ role: 'assistant', content, reasoning: '旧思考' }], next })
const originalSession = {
  id: 'chat', configId: 'provider', model: 'model', reasoningEffort: 'high',
  tree: { root: { choice: 0, next: [userNode('first-user', '第一条问题', [
    replyNode('first-reply', '第一条回答', [userNode('edited-user', '修改前的问题', [
      replyNode('old-reply', '修改前的回答', [userNode('later-user', '后续问题', [replyNode('later-reply', '后续回答')])]),
    ])]),
  ])] } },
}
let savedSession = originalSession
globalThis.window = {
  go: { Backend: { Core: {
    GetChat: async () => structuredClone(savedSession), ListChats: async () => [],
    ListPersonas: async () => [], ListWorkspaces: async () => [], GetChatModelContextWindow: async () => 10000,
    EditChatMessage: (...args) => {
      editCalls++
      editedArguments = args
      return new Promise((resolve, reject) => { completeEdit = resolve; rejectEdit = reject })
    },
  } } },
  runtime: { EventsOn: (name, handler) => listeners.set(name, handler), EventsOff: (name) => listeners.delete(name) },
}
const renderer = createRenderer({
  createElement: element, createText: (text) => ({ ...element('#text'), text }), createComment: (text) => ({ ...element('#comment'), text }),
  setText: (node, text) => { node.text = text },
  setElementText: (node, text) => { node.text = text; node.children = [] },
  patchProp: (node, key, previous, value) => { node.props[key] = value },
  insert(node, parent, anchor = null) {
    detach(node)
    node.parent = parent
    parent.children.splice(anchor ? parent.children.indexOf(anchor) : parent.children.length, 0, node)
  },
  remove: detach, parentNode: (node) => node.parent,
  nextSibling: (node) => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
  querySelector: () => body,
})
const pinia = createPinia()
setActivePinia(pinia)
const chat = useChatStore()
chat.session = structuredClone(originalSession)
chat.rebuildMessages()
const originalMessages = chat.messages.slice()
const app = renderer.createApp({ render: () => h(ChatPage) })
app.use(pinia)
app.mount(root)
const flush = async () => { await Promise.resolve(); await nextTick(); await Promise.resolve(); await nextTick() }
await flush()
const clickEdit = () => {
  const row = find(root, (node) => node.type === 'div' && node.props.class === 'msg user'
    && node.children.some((child) => child.props.class === 'msg-body' && child.text === '修改前的问题'))
  assert.ok(row, 'the user message to edit should be visible')
  find(row, (node) => node.props['aria-label'] === '编辑').props.onClick()
}
const pressEnter = (properties = {}) => {
  const event = { key: 'Enter', shiftKey: false, ctrlKey: false, altKey: false, metaKey: false, isComposing: false,
    keyCode: 13, defaultPrevented: false, preventDefault() { this.defaultPrevented = true }, ...properties }
  const handler = editInput().props.onKeydown
  for (const listener of Array.isArray(handler) ? handler : [handler]) listener(event)
  return event
}
const emit = (name, payload) => listeners.get(name)(JSON.stringify({ chatId: 'chat', nodeId: 'new-reply', ...payload }))

clickEdit()
await flush()
editInput().props['onUpdate:modelValue']('  修改后的问题  ')
pressEnter()
await flush()
assert.equal(editCalls, 1)
assert.deepEqual(editedArguments, ['chat', 'edited-user', '修改后的问题', 'high'])
assert.equal(chat.sending, true)
assert.deepEqual(chat.messages.map(({ role, content }) => ({ role, content })), [
  { role: 'user', content: '第一条问题' }, { role: 'assistant', content: '第一条回答' },
  { role: 'user', content: '修改后的问题' }, { role: 'assistant', content: '' },
], 'submitting an edit should replace the visible continuation before streaming begins')
assert.equal(editInput(), undefined, 'the editor should close while the reply is being generated')
assert.deepEqual(chat.session, originalSession, 'optimistic rendering must preserve the stored original branch')
assert.equal(originalMessages[3].content, '修改前的回答')
const pending = chat.messages.at(-1)
emit('chat:delta', { reasoning: '新思考', content: '正在读取' })
emit('chat:tool', { callId: 'read', name: '读取文件', event: 'start' })
emit('chat:tool', { callId: 'read', name: '读取文件', event: 'done' })
emit('chat:delta', { content: '新回答' })
await flush()
assert.equal(pending.content, '新回答')
assert.deepEqual(pending.process.map(({ kind, content, status }) => ({ kind, content, status })), [
  { kind: 'reasoning', content: '新思考', status: undefined },
  { kind: 'text', content: '正在读取', status: undefined },
  { kind: 'tool', content: undefined, status: 'done' },
])
assert.equal(find(root, (node) => node.props.innerHTML === '新回答')?.props.class, 'msg-body markdown-body')
assert.equal(originalMessages[5].content, '后续回答')
savedSession = structuredClone(originalSession)
const parent = savedSession.tree.root.next[0].next[0]
parent.next.push(userNode('new-user', '修改后的问题', [replyNode('new-reply', '新回答')]))
parent.choice = 1
completeEdit()
await flush()
assert.equal(chat.sending, false)
assert.equal(chat.messages[2].nodeId, 'new-user')
assert.equal(chat.messages[3].nodeId, 'new-reply')
assert.equal(chat.messages[3].content, '新回答')

savedSession = originalSession
chat.session = structuredClone(originalSession)
chat.rebuildMessages()
clickEdit()
await flush()
editInput().props['onUpdate:modelValue']('另一个问题')
assert.equal(pressEnter({ isComposing: true }).defaultPrevented, false)
assert.equal(pressEnter({ keyCode: 229 }).defaultPrevented, false)
assert.equal(pressEnter({ shiftKey: true }).defaultPrevented, false)
await flush()
assert.equal(editCalls, 1, 'IME confirmation and Shift+Enter must not submit an edit')
pressEnter()
pressEnter()
await flush()
assert.equal(editCalls, 2, 'repeated Enter must submit only once')
rejectEdit(new Error('编辑请求失败'))
await flush()
assert.equal(chat.sending, false)
assert.match(chat.error, /编辑请求失败/)
assert.deepEqual(chat.messages.map((message) => message.content), originalMessages.map((message) => message.content))
app.unmount()
console.log('Chat edit checks passed: Enter submission, immediate rendering, isolated stream/tool events, preserved branches, IME, duplicate submission, and failure restoration.')
