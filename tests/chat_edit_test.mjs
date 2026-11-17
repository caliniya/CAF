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
const vueURL = pathToFileURL(require.resolve('vue')).href
const dialogURL = dataModule(`import { h } from ${JSON.stringify(vueURL)}; export default { inheritAttrs: false, setup(_, { attrs }) { return () => h('test-dialog', { ...attrs }) } }`)
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
    getBoundingClientRect() { return { left: 120, right: 380, top: 220, bottom: 250, height: 120 } },
    querySelector(selector) { return find(this, node => selector.startsWith('.') ? String(node.props.class ?? '').split(' ').includes(selector.slice(1)) : node.props.id === selector.slice(1)) },
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
  innerWidth: 1200, innerHeight: 800,
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
let editFinished
chat.$onAction(({ name, after, onError }) => {
  if (name === 'editMessage') editFinished = new Promise((resolve, reject) => { after(resolve); onError(reject) })
})
chat.session = structuredClone(originalSession)
chat.rebuildMessages()
const originalMessages = chat.messages.slice()
const app = renderer.createApp({ render: () => h(ChatPage) })
app.use(pinia)
app.mount(root)
const flush = async () => { await new Promise((resolve) => setImmediate(resolve)); await nextTick() }
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
assert.ok(find(root, (node) => node.props.class === 'msg-body markdown-body' && node.props.innerHTML?.startsWith('新回答')))
assert.equal(originalMessages[5].content, '后续回答')
savedSession = structuredClone(originalSession)
const parent = savedSession.tree.root.next[0].next[0]
parent.next.push(userNode('new-user', '修改后的问题', [replyNode('new-reply', '新回答')]))
parent.choice = 1
completeEdit()
await editFinished
await flush()
assert.equal(chat.sending, false)
assert.equal(chat.messages[2].nodeId, 'new-user')
assert.equal(chat.messages[3].nodeId, 'new-reply')
assert.equal(chat.messages[3].content, '新回答')

savedSession = originalSession
chat.session = structuredClone(originalSession)
chat.rebuildMessages()
await flush()
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
await editFinished
await flush()
assert.equal(chat.sending, false)
assert.match(chat.error, /编辑请求失败/)
assert.deepEqual(chat.messages.map((message) => message.content), originalMessages.map((message) => message.content))

// Workspace selection uses the same confirmation path for existing and newly created workspaces.
const workspaces = [{ id: 'workspace-1', name: '项目一', path: '/one' }, { id: 'workspace-2', name: '项目二', path: '/two' }]
savedSession = { ...structuredClone(originalSession), workspaceId: 'workspace-1', memoryLibraryIds: ['global', 'local'] }
chat.session = structuredClone(savedSession)
chat.workspaces = [...workspaces]
chat.rebuildMessages()
let workspaceChanges = 0
window.go.Backend.Core.ListWorkspaces = async () => [...workspaces]
window.go.Backend.Core.SetChatWorkspace = async (id, workspaceId) => {
  workspaceChanges++
  savedSession.workspaceId = workspaceId
  savedSession.memoryLibraryIds = ['global']
  let last = savedSession.tree.root.next[savedSession.tree.root.choice]
  while (last.next?.length) last = last.next[last.choice]
  last.next = [{ id: `workspace-switch-${workspaceChanges}`, choice: 0, next: [], child: [{ role: 'system', content: '系统消息:从「项目一」切换到「项目二」' }] }]
  last.choice = 0
  return structuredClone(savedSession)
}
const nodeText = node => node.type === '#comment' ? '' : (node.text ?? '') + (node.children ?? []).map(nodeText).join('')
const dialogByTitle = title => find(root, node => node.type === 'test-dialog' && node.props.title === title)
const chooseWorkspace = async name => {
  await find(root, node => node.type === 'button' && node.props['aria-controls'] === 'workspace-menu').props.onClick()
  await flush()
  await find(root, node => node.type === 'button' && node.props['aria-controls'] === 'workspace-list').props.onClick({ currentTarget: { getBoundingClientRect: () => ({ left: 120, right: 380, top: 220 }) } })
  await flush()
  await find(root, node => node.type === 'button' && node.props.role === 'menuitemradio' && nodeText(node) === name).props.onClick()
  await flush()
}
await chooseWorkspace('项目二')
assert.equal(workspaceChanges, 0, 'selecting another workspace must wait for confirmation')
assert.equal(chat.currentWorkspaceId, 'workspace-1')
let workspaceDialog = dialogByTitle('切换工作区')
assert.equal(workspaceDialog.props.open, true)
assert.match(workspaceDialog.props.message, /项目一.*项目二[\s\S]*缓存命中率和模型工作能力/)
workspaceDialog.props.onCancel()
await flush()
assert.equal(workspaceDialog.props.open, false)
assert.equal(workspaceChanges, 0, 'cancel must preserve the original workspace')
await chooseWorkspace('项目二')
await dialogByTitle('切换工作区').props.onConfirm()
await flush()
assert.equal(workspaceChanges, 1)
assert.equal(chat.currentWorkspaceId, 'workspace-2')
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'])
assert.equal(chat.messages.at(-1).role, 'system')
assert.ok(find(root, node => node.props.class === 'msg-system-body' && node.text === '系统消息:从「项目一」切换到「项目二」'), 'workspace switch should be visible as plain system text')
await chooseWorkspace('项目二')
assert.equal(workspaceChanges, 1, 'reselecting the current workspace must do nothing')
assert.equal(dialogByTitle('切换工作区').props.open, false)

window.go.Backend.Core.CreateWorkspace = async (name, workspacePath) => ({ id: 'new-workspace', name, path: workspacePath })
await find(root, node => node.type === 'button' && node.props['aria-controls'] === 'workspace-menu').props.onClick()
await flush()
find(root, node => node.type === 'button' && nodeText(node) === '新建工作区').props.onClick()
await flush()
const createWorkspaceDialog = find(root, node => node.type === 'test-dialog' && 'initial-directory' in node.props)
await createWorkspaceDialog.props.onConfirm('新项目', '/new')
await flush()
assert.equal(workspaceChanges, 1, 'creating a workspace must not bypass the switch confirmation')
assert.equal(dialogByTitle('切换工作区').props.open, true)
dialogByTitle('切换工作区').props.onCancel()
await flush()

// Memory management actions only reach the host after the required second dialog confirms.
let memoryLibraries = [
  { id: 'global', name: 'Main', scope: 'global' },
  { id: 'local', name: '项目笔记', scope: 'workspace', workspaceId: 'workspace-2' },
  { id: 'shared', name: '通用笔记', scope: 'global' },
]
window.go.Backend.Core.GetChatMemorySettings = async () => ({ libraries: [...memoryLibraries], enabledIds: ['global'] })
let deleteCalls = 0, scopeCalls = 0, renameArguments, createArguments
window.go.Backend.Core.DeleteMemoryLibrary = async (chatId, workspaceId, id) => {
  deleteCalls++
  assert.deepEqual([chatId, workspaceId, id], ['chat', 'workspace-2', 'local'])
  memoryLibraries = memoryLibraries.filter(library => library.id !== id)
}
window.go.Backend.Core.SetMemoryLibraryScope = async (chatId, workspaceId, id, scope) => {
  scopeCalls++
  assert.deepEqual([chatId, workspaceId, id, scope], ['chat', 'workspace-2', 'shared', 'workspace'])
  memoryLibraries = memoryLibraries.map(library => library.id === id ? { ...library, scope, workspaceId } : library)
}
window.go.Backend.Core.RenameMemoryLibrary = async (...args) => { renameArguments = args }
window.go.Backend.Core.CreateMemoryLibrary = async (...args) => {
  createArguments = args
  memoryLibraries.push({ id: 'created', name: args[2], scope: args[3], workspaceId: args[1] })
}
find(root, node => node.type === 'button' && node.props['aria-controls'] === 'chat-settings-menu').props.onClick()
await flush()
await find(root, node => node.type === 'button' && node.props.role === 'menuitem' && nodeText(node).startsWith('记忆设置')).props.onClick()
await flush()
const memoryDialog = find(root, node => node.type === 'test-dialog' && Array.isArray(node.props.libraries))
assert.equal(memoryDialog.props.open, true)
memoryDialog.props.onDelete(memoryLibraries[1])
await flush()
assert.equal(deleteCalls, 0)
assert.equal(dialogByTitle('删除记忆').props.open, true)
dialogByTitle('删除记忆').props.onCancel()
await flush()
assert.equal(deleteCalls, 0)
memoryDialog.props.onDelete(memoryLibraries[1])
await flush()
await dialogByTitle('删除记忆').props.onConfirm()
await flush()
assert.equal(deleteCalls, 1)
assert.ok(memoryDialog.props.libraries.every(library => library.id !== 'local'))

memoryDialog.props.onScope(memoryLibraries.find(library => library.id === 'shared'))
await flush()
assert.equal(scopeCalls, 0)
assert.equal(dialogByTitle('更改作用范围').props.open, true)
assert.match(dialogByTitle('更改作用范围').props.message, /项目二[\s\S]*其他工作区将无法看见/)
await dialogByTitle('更改作用范围').props.onConfirm()
await flush()
assert.equal(scopeCalls, 1)
memoryDialog.props.onCreate()
await flush()
const memoryForm = find(root, node => node.type === 'test-dialog' && node.props.mode === 'create')
assert.equal(memoryForm.props.open, true)
await memoryForm.props.onConfirm('新记忆', 'workspace')
await flush()
assert.deepEqual(createArguments, ['chat', 'workspace-2', '新记忆', 'workspace'])
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'], 'creating memory should not silently enable it')
memoryDialog.props.onRename(memoryLibraries.find(library => library.id === 'created'))
await flush()
await find(root, node => node.type === 'test-dialog' && node.props.mode === 'rename').props.onConfirm('新名称', 'workspace')
await flush()
assert.deepEqual(renameArguments, ['chat', 'workspace-2', 'created', '新名称'])

app.unmount()
console.log('Chat edit and settings checks passed: editing, streaming, IME, workspace switch confirmation, system messages, memory management confirmation, and create/rename actions.')
