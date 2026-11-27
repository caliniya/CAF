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
const { createRenderer, h, KeepAlive, nextTick, ref } = await import(pathToFileURL(require.resolve('vue')).href)
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
await loginTestProfile(apiURL)
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
const pageActive = ref(true)
const app = renderer.createApp({ render: () => h(KeepAlive, null, { default: () => pageActive.value ? h(ChatPage) : null }) })
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
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global', 'local'], 'workspace switches must preserve selected libraries')
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
  { id: 'local', name: '项目笔记', scope: 'workspace', workspaceId: 'workspace-1' },
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
const showMemorySettings = async () => {
  find(root, node => node.type === 'button' && node.props['aria-controls'] === 'chat-settings-menu').props.onClick()
  await flush()
  await find(root, node => node.type === 'button' && node.props.role === 'menuitem' && nodeText(node).startsWith('记忆设置')).props.onClick()
  await flush()
}
await showMemorySettings()
const memoryDialog = find(root, node => node.type === 'test-dialog' && Array.isArray(node.props.libraries))
assert.equal(memoryDialog.props.open, true)
assert.deepEqual(memoryDialog.props.workspaces, workspaces, 'settings must receive the current workspace names')

// Information is lazy-loaded even for disabled libraries; late responses stay with their dialog.
const detailsRequests = []
window.go.Backend.Core.GetMemoryLibraryDetails = (...args) => new Promise((resolve, reject) => {
  detailsRequests.push({ args, resolve, reject })
})
const memoryDetailsDialog = find(root, node => node.type === 'test-dialog' && 'details' in node.props && 'library' in node.props)
const detailsSnapshot = library => ({ ...library, description: '详情介绍', data: {
  'memory-test': { name: '当前记忆', data: '当前记忆正文', description: ['标签'], date: '2026-11-18T06:00:00Z' },
} })
const firstDetailsLoad = memoryDialog.props.onInfo(memoryLibraries[1])
await flush()
assert.deepEqual(detailsRequests[0].args, ['chat', 'workspace-2', 'local'])
assert.equal(memoryDetailsDialog.props.library.id, 'local')
assert.equal(memoryDetailsDialog.props.loading, true)
assert.equal(memoryDetailsDialog.props.details, null)
assert.deepEqual(memoryDetailsDialog.props.workspaces, workspaces, 'details must receive the same workspace names')
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'], 'viewing disabled memory must not enable it')
memoryDetailsDialog.props.onClose()
await flush()
assert.equal(memoryDetailsDialog.props.library, null)
assert.equal(memoryDetailsDialog.props.loading, false)
assert.equal(memoryDialog.props.open, true, 'closing details should preserve the settings dialog')
const secondDetailsLoad = memoryDialog.props.onInfo(memoryLibraries[0])
await flush()
detailsRequests[1].resolve(detailsSnapshot(memoryLibraries[0]))
await secondDetailsLoad
await flush()
detailsRequests[0].resolve(detailsSnapshot(memoryLibraries[1]))
await firstDetailsLoad
await flush()
assert.equal(memoryDetailsDialog.props.details.id, 'global', 'a response from a closed library must not overwrite the current details')
assert.equal(memoryDetailsDialog.props.loading, false)

// Entry management must use the current library and wait for explicit form/confirmation actions.
let createEntryArgs, updateEntryArgs, deleteEntryArgs, deletedEntries = 0
window.go.Backend.Core.CreateMemoryEntry = async (...args) => {
  createEntryArgs = args
  return { ...detailsSnapshot(memoryLibraries[0]), data: {
    'memory-created': { name: args[3].name, data: args[3].data, description: args[3].tags, date: '2026-11-18T09:00:00Z' },
  } }
}
window.go.Backend.Core.UpdateMemoryEntry = async (...args) => {
  updateEntryArgs = args
  return { ...memoryDetailsDialog.props.details, data: {
    ...memoryDetailsDialog.props.details.data,
    [args[3]]: { name: args[4].name, data: args[4].data, description: args[4].tags, date: args[4].date },
  } }
}
window.go.Backend.Core.DeleteMemoryEntry = async (...args) => {
  deleteEntryArgs = args
  deletedEntries++
  return { ...memoryDetailsDialog.props.details, data: {} }
}
const entryForm = () => find(root, node => node.type === 'test-dialog' && 'memory-id' in node.props)
memoryDetailsDialog.props.onCreate()
await flush()
assert.equal(entryForm().props.open, true)
assert.equal(entryForm().props.mode, 'create')
assert.equal(createEntryArgs, undefined, 'opening creation must not persist an entry')
const createdInput = { name: '新记忆', data: '手动填写正文', tags: ['标签'] }
await entryForm().props.onConfirm(createdInput)
await flush()
assert.deepEqual(createEntryArgs, ['chat', 'workspace-2', 'global', createdInput])
assert.equal(entryForm().props.open, false)
assert.equal(memoryDetailsDialog.props.details.data['memory-created'].name, '新记忆')
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'], 'entry creation must not change library selection')
memoryDetailsDialog.props.onEdit('memory-created')
await flush()
assert.equal(entryForm().props.open, true)
assert.equal(entryForm().props.mode, 'edit')
assert.equal(entryForm().props['memory-id'], 'memory-created')
assert.equal(entryForm().props.entry.name, '新记忆')
const editedInput = { name: '已改名', data: '编辑后正文', date: '2026-11-17T08:00:00Z', tags: [] }
await entryForm().props.onConfirm(editedInput)
await flush()
assert.deepEqual(updateEntryArgs, ['chat', 'workspace-2', 'global', 'memory-created', editedInput])
assert.equal(memoryDetailsDialog.props.details.data['memory-created'].name, '已改名')
memoryDetailsDialog.props.onDelete('memory-created')
await flush()
assert.equal(deletedEntries, 0, 'opening deletion must wait for a second confirmation')
assert.equal(dialogByTitle('删除单条记忆').props.open, true)
assert.match(dialogByTitle('删除单条记忆').props.message, /已改名[\s\S]*历史/)
dialogByTitle('删除单条记忆').props.onCancel()
await flush()
assert.equal(deletedEntries, 0)
assert.ok(memoryDetailsDialog.props.details.data['memory-created'])
memoryDetailsDialog.props.onDelete('memory-created')
await flush()
await dialogByTitle('删除单条记忆').props.onConfirm()
await flush()
assert.deepEqual(deleteEntryArgs, ['chat', 'workspace-2', 'global', 'memory-created'])
assert.equal(deletedEntries, 1)
assert.equal(dialogByTitle('删除单条记忆').props.open, false)
assert.deepEqual(memoryDetailsDialog.props.details.data, {})

memoryDetailsDialog.props.onCreate()
await flush()
window.go.Backend.Core.CreateMemoryEntry = async () => { throw new Error('记忆保存失败') }
await entryForm().props.onConfirm(createdInput)
await flush()
assert.equal(entryForm().props.open, true, 'failed saves must preserve the open form')
assert.match(entryForm().props.error, /记忆保存失败/)
entryForm().props.onCancel()
await flush()
assert.equal(entryForm().props.open, false)

// A save from another conversation must not reopen a form or replace current details.
let finishOldEntrySave
window.go.Backend.Core.CreateMemoryEntry = () => new Promise(resolve => { finishOldEntrySave = resolve })
memoryDetailsDialog.props.onCreate()
await flush()
const oldEntrySave = entryForm().props.onConfirm(createdInput)
await flush()
assert.equal(entryForm().props.busy, true)
assert.equal(memoryDetailsDialog.props.busy, true)
memoryDetailsDialog.props.onClose()
await flush()
assert.equal(memoryDetailsDialog.props.library.id, 'global', 'a pending save must keep its dialog open')
chat.session = { ...chat.session, id: 'other-entry-chat' }
await flush()
assert.equal(entryForm().props.open, false)
assert.equal(memoryDetailsDialog.props.library, null)
finishOldEntrySave(detailsSnapshot(memoryLibraries[0]))
await oldEntrySave
await flush()
assert.equal(memoryDetailsDialog.props.details, null)
assert.equal(entryForm().props.busy, false)
chat.session = { ...chat.session, id: 'chat' }
await flush()
await showMemorySettings()
memoryDetailsDialog.props.onClose()

const failedDetailsLoad = memoryDialog.props.onInfo(memoryLibraries[2])
await flush()
detailsRequests[2].reject(new Error('详情读取失败'))
await failedDetailsLoad
await flush()
assert.match(memoryDetailsDialog.props.error, /详情读取失败/)
assert.equal(memoryDetailsDialog.props.loading, false)
const retryDetailsLoad = memoryDetailsDialog.props.onRetry()
await flush()
assert.equal(memoryDetailsDialog.props.error, '')
assert.equal(memoryDetailsDialog.props.loading, true)
assert.deepEqual(detailsRequests[3].args, ['chat', 'workspace-2', 'shared'])
detailsRequests[3].resolve(detailsSnapshot(memoryLibraries[2]))
await retryDetailsLoad
await flush()
assert.equal(memoryDetailsDialog.props.details.id, 'shared')
memoryDetailsDialog.props.onClose()
await flush()

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
assert.match(dialogByTitle('更改作用范围').props.message, /项目二[\s\S]*模型只能在使用该工作区的对话中写入/)
await dialogByTitle('更改作用范围').props.onConfirm()
await flush()
assert.equal(scopeCalls, 1)
memoryDialog.props.onCreate()
await flush()
const memoryForm = find(root, node => node.type === 'test-dialog' && node.props.mode === 'create' && 'initial-name' in node.props)
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

const oldChatLoad = memoryDialog.props.onInfo(memoryLibraries[0])
await flush()
chat.session = { ...chat.session, id: 'another-chat' }
await flush()
assert.equal(memoryDialog.props.open, false)
assert.equal(memoryDetailsDialog.props.library, null)
detailsRequests[4].resolve(detailsSnapshot(memoryLibraries[0]))
await oldChatLoad
await flush()
assert.equal(memoryDetailsDialog.props.details, null, 'a previous conversation must not restore closed details')

chat.session = { ...chat.session, id: 'chat' }
await flush()
await showMemorySettings()
const oldWorkspaceLoad = memoryDialog.props.onInfo(memoryLibraries[0])
await flush()
chat.session.workspaceId = 'workspace-1'
await flush()
assert.equal(memoryDialog.props.open, false)
assert.equal(memoryDetailsDialog.props.library, null)
detailsRequests[5].reject(new Error('旧工作区的详情读取失败'))
await oldWorkspaceLoad
await flush()
assert.equal(memoryDetailsDialog.props.error, '', 'a previous workspace error must not affect the current dialog')

await showMemorySettings()
const closedSettingsLoad = memoryDialog.props.onInfo(memoryLibraries[0])
await flush()
memoryDialog.props.onCancel()
await flush()
assert.equal(memoryDetailsDialog.props.library, null)
detailsRequests[6].resolve(detailsSnapshot(memoryLibraries[0]))
await closedSettingsLoad
await flush()
assert.equal(memoryDetailsDialog.props.details, null, 'closing settings must invalidate pending detail loads')

await showMemorySettings()
const deactivatedDetailsLoad = memoryDialog.props.onInfo(memoryLibraries[0])
await flush()
pageActive.value = false
await flush()
detailsRequests[7].resolve(detailsSnapshot(memoryLibraries[0]))
await deactivatedDetailsLoad
pageActive.value = true
await flush()
assert.equal(memoryDialog.props.open, false)
assert.equal(memoryDetailsDialog.props.library, null, 'leaving the page must close details and discard their pending response')

// Strict mode uses the existing persisted tool config and preserves unrelated switches and grants.
const toolConfigCalls = []
window.go.Backend.Core.SetChatToolConfig = async (id, raw) => {
  const config = JSON.parse(raw)
  toolConfigCalls.push([id, config])
  return { ...chat.session, toolConfig: config }
}
chat.session.toolConfig = { permission: 1, sets: { '主工具集': { kits: { '文件操作工具': false } } }, allowedTools: { WriteFile: { scope: 'workspace', workspaceId: 'workspace-1' } } }
await flush()
const openPermissions = async () => {
  await find(root, node => node.type === 'button' && node.props.class?.includes('perm-trigger')).props.onClick()
  await flush()
}
const strictToggle = () => find(root, node => node.type === 'button' && node.props.role === 'menuitemcheckbox' && nodeText(node).includes('启用严格模式'))
await openPermissions()
assert.equal(strictToggle().props['aria-checked'], false)
await strictToggle().props.onClick()
await flush()
assert.equal(chat.strictMode, true)
assert.equal(toolConfigCalls.at(-1)[1].strictMode, true)
assert.equal(toolConfigCalls.at(-1)[1].sets['主工具集'].kits['文件操作工具'], false)
assert.equal(toolConfigCalls.at(-1)[1].allowedTools.WriteFile.scope, 'workspace')
await openPermissions()
assert.equal(strictToggle().props['aria-checked'], true)
chat.sending = true
await flush()
assert.equal(strictToggle().props.disabled, true)
chat.sending = false
await flush()
await strictToggle().props.onClick()
await flush()
assert.equal(chat.strictMode, false)
assert.equal(toolConfigCalls.at(-1)[1].strictMode, false)
await openPermissions()
const clearGrants = find(root, node => node.type === 'button' && nodeText(node).includes('清除工具授权'))
assert.ok(clearGrants)
await clearGrants.props.onClick()
await flush()
assert.equal(toolConfigCalls.at(-1)[1].allowedTools, undefined)
chat.session.toolConfig.allowedTools = { WriteFile: { scope: 'tool' } }
chat.session.toolConfig.permission = 4
await chat.setPermission(1)
await flush()
assert.equal(toolConfigCalls.at(-1)[1].allowedTools, undefined, 'lowering permissions must revoke previous grants')

// Compression controls call the active chat only and preserve the settings for a one-off run.
const coreApi = window.go.Backend.Core
const compressionCalls = []
coreApi.SetChatCompactionMode = async (id, mode) => { compressionCalls.push([id,mode]); return { ...chat.session, compactionMode: mode ?? undefined } }
coreApi.CompactChat = async id => { compressionCalls.push([id,'compact']); return { ...chat.session } }
const settingButton = find(root, node => node.props['aria-controls'] === 'chat-settings-menu')
await settingButton.props.onClick(); await flush()
const submenuButton = find(root, node => node.props['aria-controls'] === 'chat-compaction-menu')
await submenuButton.props.onClick({ currentTarget: submenuButton }); await flush()
const submenu = () => find(root, node => node.props.id === 'chat-compaction-menu')
const compressionAction = label => find(submenu(), node => node.type === 'button' && nodeText(node) === label)
assert.equal(compressionAction('自动压缩上下文').props['aria-checked'], false)
await compressionAction('自动压缩上下文').props.onClick(); await flush()
assert.deepEqual(compressionCalls.at(-1), [chat.session.id, { enabled:true, automatic:true }])
await compressionAction('启用上下文压缩').props.onClick(); await flush()
assert.equal(compressionAction('自动压缩上下文').props.disabled,true)
await compressionAction('立即压缩上下文').props.onClick(); await flush()
assert.deepEqual(compressionCalls.at(-1), [chat.session.id,'compact'])
assert.equal(chat.session.compactionMode.enabled,false)
let decision
coreApi.SubmitCompactionDecision = async (...args) => { decision = args }
const activeId = chat.session.id
emit('chat:compaction', {chatId:activeId, requestId:'first', used:35500,threshold:30000,nextThreshold:60000})
await flush()
const card = find(root, node => node.props['aria-label'] === '上下文压缩提醒')
assert.ok(card && nodeText(card).includes('60,000'))
await find(card,node => node.type==='button' && nodeText(node)==='暂不压缩').props.onClick();await flush()
assert.deepEqual(decision,[activeId,'first',false]);assert.equal(chat.compactionPrompt,undefined)
emit('chat:compaction',{chatId:'another', requestId:'hidden', used:60000, threshold:60000});await flush()
assert.equal(find(root,node=>node.props['aria-label']==='上下文压缩提醒'),undefined)

// Paste an image without a prompt, send it through the real store/bridge, and stop before the first token.
const samplePNG = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aN0cAAAAASUVORK5CYII='
const screenshot = new File([Buffer.from(samplePNG, 'base64')], '截图.png', {type: 'image/png'})
globalThis.FileReader = class {
  async readAsDataURL(file) { this.result = `data:${file.type};base64,${Buffer.from(await file.arrayBuffer()).toString('base64')}`; this.onload() }
}
let sentImage, finishImageSend, stopTarget
coreApi.SendChatMessageWithAttachments = (...args) => {
  sentImage = args
  return new Promise(resolve => { finishImageSend = resolve })
}
coreApi.StopChatGeneration = async (...args) => {
  stopTarget = args
  const stopped = {id: 'image-reply', choice: -1, child: [], stopped: true}
  const user = userNode('image-user', '', [stopped])
  user.child[0].attachments = [{id: 'attachment-demo', name: '截图.png', mime: 'image/png'}]
  savedSession = {...originalSession, id: activeId, tree: {root: {choice: 0, next: [user]}}}
  finishImageSend()
}
const composer = find(root, node => node.props.class === 'composer')
const composerAction = label => find(composer, node => node.type === 'button' && node.props['aria-label'] === label)
chat.draft = ''
let handledPaste = false
composer.props.onPaste({clipboardData: {files: [screenshot]}, preventDefault() { handledPaste = true }})
await flush()
assert.ok(handledPaste && composerAction('移除 截图.png'))
assert.equal(composerAction('发送').props.disabled, false)
const sendingImage = composerAction('发送').props.onClick()
await flush()
assert.deepEqual(sentImage, [activeId, '', chat.effort, [{name: '截图.png', data: samplePNG}]])
emit('chat:generation', {chatId: activeId, nodeId: 'image-reply'})
await flush()
assert.ok(composerAction('停止生成'))
chat.session = {...chat.session, id: 'other-image-chat'}
await flush()
assert.equal(composerAction('停止生成'), undefined, 'switching chats must hide the other reply’s stop button')
chat.session = {...chat.session, id: activeId}
await flush()
await composerAction('停止生成').props.onClick()
await sendingImage
await flush()
assert.deepEqual(stopTarget, [activeId, 'image-reply'])
assert.ok(find(root, node => node.type === 'p' && nodeText(node) === '已停止生成'))
assert.equal(chat.messages.at(-1).stopped, true, 'a stopped node with no deltas must remain visible')
assert.equal(chat.sending, false)
assert.deepEqual(chat.draftFiles, [])

app.unmount()
const actualConfigsURL = dataModule(rewrite((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/stores/configs.ts'), 'utf8'), 'configs.ts')).code))
imports['../stores/configs'] = actualConfigsURL
imports['../memory'] = dataModule((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/memory.ts'), 'utf8'), 'memory.ts')).code)
const settingsFile = path.join(frontend, 'src/pages/SettingsChatPage.vue')
const settingsScript = compileScript(parse(fs.readFileSync(settingsFile,'utf8')).descriptor, { id: 'chat-defaults-check', inlineTemplate:true })
const { default: SettingsChatPage } = await import(dataModule(rewrite((await transformWithOxc(settingsScript.content,'SettingsChatPage.ts')).code)))
const savedDefaults = { tools:{ sets:{ demo:{kits:{read:false}} } }, memoryLibraryIds:['main'], compaction:{enabled:true,automatic:false,windowPercent:70,tokenThreshold:500000} }
let globalPayload
coreApi.GetChatDefaults = async () => structuredClone(savedDefaults)
coreApi.SetChatDefaults = async value => { globalPayload=JSON.parse(JSON.stringify(value)); return JSON.parse(JSON.stringify(value)) }
coreApi.ListToolSets = async () => [{id:'demo', name:'工具',kits:[{id:'read',name:'读取',defaultEnabled:true}]}]
coreApi.GetChatMemorySettings = async () => ({ libraries:[{id:'main',name:'Main',scope:'global'}], enabledIds:['main'] })
const prefsApp = renderer.createApp(SettingsChatPage)
prefsApp.use(pinia);prefsApp.mount(root);await flush()
const globalStore=(await import(actualConfigsURL)).useConfigsStore()
const settingInput = label => find(find(root,node=>node.type==='label' && nodeText(node).startsWith(label)),node=>node.type==='input')
const globalForm = find(root,node=>node.type==='form')
settingInput('模型上下文窗口比例').props['onUpdate:modelValue'](55)
settingInput('固定 token 阈值').props['onUpdate:modelValue'](300000)
settingInput('全局主记忆').props['onUpdate:modelValue']([])
settingInput('读取').props.onChange({ target:{ checked:true } })
settingInput('自动压缩上下文').props['onUpdate:modelValue'](true)
await flush()
assert.equal(globalStore.defaultCompaction.windowPercent,70,'unsaved thresholds leaked into active chats')
assert.equal(globalStore.toolNormalConfig.sets.demo.kits.read,false,'unsaved module selection leaked into active chats')
await globalForm.props.onSubmit({ preventDefault(){} });await flush()
assert.deepEqual(globalPayload.compaction,{enabled:true,automatic:true,windowPercent:55,tokenThreshold:300000})
assert.deepEqual(globalStore.defaultMemoryIds,[])
assert.equal(globalStore.toolNormalConfig.sets,undefined)
assert.equal(globalStore.defaultCompaction.tokenThreshold,300000)
prefsApp.unmount()
console.log('Chat edit and settings checks passed, including global defaults, compression menus, in-chat decisions, and isolated drafts.')
