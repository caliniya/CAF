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
const { createRenderer, h, nextTick, reactive } = await import(pathToFileURL(require.resolve('vue')).href)
const { createPinia, setActivePinia } = await import(pathToFileURL(require.resolve('pinia')).href)
const dataModule = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const apiURL = dataModule((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/api.ts'), 'utf8'), 'api.ts')).code)
const memoryURL = dataModule((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/memory.ts'), 'utf8'), 'memory.ts')).code)
const configsURL = dataModule('const state = { enables: { ID: "provider" }, enabledModel: "model", defaultMemoryIds: [] }; export const useConfigsStore = () => state')
function rewrite(code) {
  return code.replace(/from\s+(['"])([^'"]+)\1/g, (_whole, _quote, specifier) => {
    const url = specifier === '../api' ? apiURL : specifier === '../memory' ? memoryURL : specifier === './configs' ? configsURL : pathToFileURL(require.resolve(specifier)).href
    return `from ${JSON.stringify(url)}`
  })
}
const storeURL = dataModule(rewrite((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/stores/chat.ts'), 'utf8'), 'chat.ts')).code))
const { useChatStore } = await import(storeURL)
setActivePinia(createPinia())
const chat = useChatStore()
let resolveSave, resolveLoad
let savedIDs = []
const libraries = [
  { id: 'global', name: 'Main', description: '全局记忆库', scope: 'global' },
  { id: 'local', name: '项目', scope: 'workspace', workspaceId: 'workspace-1' },
  { id: 'other-local', name: '研究笔记', scope: 'workspace', workspaceId: 'workspace-2' },
]
const workspaces = [{ id: 'workspace-1', name: '开发项目' }, { id: 'workspace-2', name: '研究项目' }]
globalThis.window = { innerWidth: 1100, innerHeight: 800, go: { Backend: { Core: {
  GetChatMemorySettings: async () => ({ libraries, enabledIds: ['global'] }),
  GetWorkspaceMemorySettings: async () => ({ libraries, enabledIds: [] }),
  SetChatMemoryLibraries: (id, ids) => {
    savedIDs = [...ids]
    return new Promise(resolve => { resolveSave = () => resolve({ id, memoryLibraryIds: ids }) })
  },
  CreateChat: async () => ({ id: 'new-chat', tree: { root: null } }),
  ListChats: async () => [],
} } } }

await loginTestProfile(apiURL)
chat.session = { id: 'chat-1', memoryLibraryIds: ['missing'] }
await chat.loadMemorySettings()
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'])
const saving = chat.setMemoryLibraries(['local'])
await nextTick()
await new Promise(setImmediate)
chat.session = { id: 'chat-2', memoryLibraryIds: [] }
resolveSave()
await saving
assert.deepEqual(chat.enabledMemoryLibraryIds, [], 'save response for an older chat must not overwrite new chat')

window.go.Backend.Core.GetChatMemorySettings = () => new Promise(resolve => { resolveLoad = resolve })
const loading = chat.loadMemorySettings()
await new Promise(setImmediate)
chat.session = { id: 'chat-3', memoryLibraryIds: ['local'] }
resolveLoad({ libraries, enabledIds: ['global'] })
await loading
assert.deepEqual(chat.enabledMemoryLibraryIds, ['local'], 'old load must not replace current chat selection')

chat.sending = true
await assert.rejects(chat.setMemoryLibraries(['global']), /生成中/)
chat.sending = false
chat.reset()
assert.deepEqual(chat.enabledMemoryLibraryIds, [])
await chat.setMemoryLibraries(['local', 'local'])
assert.deepEqual(chat.pendingMemoryLibraryIds, ['local'])
window.go.Backend.Core.SetChatMemoryLibraries = async (id, ids) => {
  savedIDs = [...ids]
  return { id, memoryLibraryIds: ids, tree: { root: null } }
}
await chat.createChat()
assert.deepEqual(savedIDs, ['local'])
assert.deepEqual(chat.session.memoryLibraryIds, ['local'])
assert.equal(chat.pendingMemoryLibraryIds, null)

// Untouched drafts inherit defaults, whereas explicitly clearing every check overrides them.
const { useConfigsStore } = await import(configsURL)
useConfigsStore().defaultMemoryIds = ['global']
chat.reset()
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'])
await chat.setMemoryLibraries([])
await chat.createChat()
assert.deepEqual(savedIDs, [])
assert.deepEqual(chat.enabledMemoryLibraryIds, [])
useConfigsStore().defaultMemoryIds = []

// Workspace binding must happen before saved draft memory IDs are checked by the backend.
const creationOrder = []
chat.reset()
chat.pendingWorkspaceId = 'workspace-1'
chat.pendingMemoryLibraryIds = ['global', 'local']
window.go.Backend.Core.SetChatWorkspace = async (id, workspaceId) => {
  creationOrder.push('workspace')
  return { id, workspaceId, tree: { root: null } }
}
window.go.Backend.Core.SetChatMemoryLibraries = async (id, ids) => {
  creationOrder.push('memory')
  return { id, workspaceId: 'workspace-1', memoryLibraryIds: ids, tree: { root: null } }
}
await chat.createChat()
assert.deepEqual(creationOrder, ['workspace', 'memory'])
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global', 'local'])

// A draft changing workspaces preserves every existing selection and removes only missing IDs.
chat.reset()
chat.pendingWorkspaceId = 'workspace-1'
chat.pendingMemoryLibraryIds = ['global', 'local', 'other-local', 'missing']
await chat.selectWorkspace('workspace-2')
assert.deepEqual(chat.pendingMemoryLibraryIds, ['global', 'local', 'other-local'])
assert.equal(chat.currentWorkspaceId, 'workspace-2')
await chat.selectWorkspace('')
assert.deepEqual(chat.pendingMemoryLibraryIds, ['global', 'local', 'other-local'])
assert.equal(chat.currentWorkspaceId, '')

// Management calls carry the actual conversation/workspace context and refresh deleted selections.
chat.session = { id: 'managed-chat', workspaceId: 'workspace-1', memoryLibraryIds: ['global', 'local'] }
let managedArguments
window.go.Backend.Core.DeleteMemoryLibrary = async (...args) => { managedArguments = args }
window.go.Backend.Core.GetChatMemorySettings = async () => ({ libraries: [libraries[0]], enabledIds: ['global'] })
const refreshed = await chat.manageMemoryLibrary({ kind: 'delete', id: 'local' })
assert.deepEqual(managedArguments, ['managed-chat', 'workspace-1', 'local'])
assert.deepEqual(refreshed.enabledIds, ['global'])
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'])

window.go.Backend.Core.RenameMemoryLibrary = () => new Promise(resolve => { resolveSave = resolve })
const rename = chat.manageMemoryLibrary({ kind: 'rename', id: 'global', name: 'Renamed' })
await new Promise(setImmediate)
chat.session = { id: 'unrelated-chat', memoryLibraryIds: ['unrelated'] }
resolveSave()
await rename
assert.deepEqual(chat.enabledMemoryLibraryIds, ['unrelated'], 'old management refresh must not overwrite another conversation')

function element(type) {
  return { type, tagName: type.toUpperCase(), props: {}, children: [], parent: null, listeners: new Map(), checked: false, open: false,
    addEventListener(event, callback) { this.listeners.set(event, callback) },
    removeEventListener() {},
    showModal() { this.open = true }, close() { this.open = false },
    getRootNode() { return document },
    get options() { return this.children.filter(node => node.type === 'option') },
  }
}
function detach(node) {
  if (!node.parent) return
  node.parent.children.splice(node.parent.children.indexOf(node), 1)
  node.parent = null
}
function find(node, predicate) {
  if (predicate(node)) return node
  for (const child of node.children ?? []) { const found = find(child, predicate); if (found) return found }
}
function text(node) { return node.type === '#comment' ? '' : (node.text ?? '') + (node.children ?? []).map(text).join('') }
const renderer = createRenderer({
  createElement: element, createText: value => ({ ...element('#text'), text: value }), createComment: value => ({ ...element('#comment'), text: value }),
  setText: (node, value) => { node.text = value }, setElementText: (node, value) => { node.text = value; node.children = [] },
  patchProp: (node, key, _old, value) => {
    node.props[key] = value
    if (key === 'value') node._value = value
    if (key === 'open') node.open = value != null && value !== false
  },
  insert(node, parent, anchor = null) { detach(node); node.parent = parent; parent.children.splice(anchor ? parent.children.indexOf(anchor) : parent.children.length, 0, node) },
  remove: detach, parentNode: node => node.parent,
  nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
  querySelector: () => body,
})
const { descriptor } = parse(fs.readFileSync(path.join(frontend, 'src/components/MemorySelectionDialog.vue'), 'utf8'))
const script = compileScript(descriptor, { id: 'memory-dialog-test', inlineTemplate: true })
const { default: Dialog } = await import(dataModule(rewrite((await transformWithOxc(script.content, 'MemorySelectionDialog.ts')).code)))
const props = reactive({ open: true, libraries, enabledIds: ['global', 'missing'], loading: false, saving: false, managing: false, workspaceId: 'workspace-1', workspaces, loadFailed: false, error: '' })
const root = element('root')
const body = element('body')
globalThis.Document = class {}
globalThis.ShadowRoot = class {}
globalThis.document = Object.assign(new Document(), { activeElement: null })
let confirmed, cancelled = 0, managementAction, infoLibrary
const app = renderer.createApp({ render: () => h(Dialog, { ...props,
  onConfirm: ids => { confirmed = ids }, onCancel: () => { cancelled++ },
  onCreate: () => { managementAction = 'create' },
  onRename: library => { managementAction = ['rename', library.id] },
  onDelete: library => { managementAction = ['delete', library.id] },
  onScope: library => { managementAction = ['scope', library.id] },
  onInfo: library => { infoLibrary = library },
}) })
app.mount(root)
await nextTick()
await nextTick()
const button = label => find(root, node => node.type === 'button' && text(node) === label)
const dialog = find(root, node => node.type === 'dialog')
assert.equal(dialog.open, true)
assert.ok(text(root).includes('记忆设置') && text(root).includes('全局') && text(root).includes('工作区'))
assert.ok(text(root).includes('全局主记忆') && text(root).includes('全局记忆') && text(root).includes('工作区记忆'))
assert.ok(text(root).includes('开发项目工作区记忆') && text(root).includes('研究项目工作区记忆'), 'workspace labels must use their owning workspace names')
assert.ok(!text(root).includes('Main') && !text(root).includes('记忆库'), 'internal name and scope labels should not appear in the interface')
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, ['global'], 'missing saved ID must not remain selected')

button('全选').props.onClick()
await nextTick()
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, ['global', 'local', 'other-local'], 'all workspace libraries should be selectable')
button('全部禁用').props.onClick()
await nextTick()
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, [], 'saving no selection must be allowed')

// Exercise an actual checkbox change, not only the bulk controls.
const checkbox = find(root, node => node.type === 'input' && node._value === 'local')
checkbox.checked = true
checkbox.listeners.get('change')()
await nextTick()
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, ['local'])

button('创建记忆').props.onClick()
assert.equal(managementAction, 'create')
const menuButton = label => find(root, node => node.type === 'button' && node.props['aria-label'] === label)
const clickMenu = async label => {
  menuButton(label).props.onClick({ currentTarget: { getBoundingClientRect: () => ({ top: 220, bottom: 250, right: 480 }) } })
  await nextTick()
}
await clickMenu('全局主记忆的选项')
assert.equal(button('重命名').props.disabled, true)
assert.equal(button('删除').props.disabled, true)
assert.equal(button('更改作用范围').props.disabled, true)
await clickMenu('项目的选项')
const floatingMenu = find(root, node => node.props.role === 'menu')
assert.equal(floatingMenu.parent, dialog, 'the library menu must sit outside the scrolling list')
dialog.props.onPointerdown({ target: { closest: selector => selector.includes('.memory-menu') ? floatingMenu : null } })
await nextTick()
assert.ok(find(root, node => node.props.role === 'menu'), 'clicking an option must not close the menu before the action')
button('重命名').props.onClick()
assert.deepEqual(managementAction, ['rename', 'local'])
await nextTick()
await clickMenu('项目的选项')
button('删除').props.onClick()
assert.deepEqual(managementAction, ['delete', 'local'])
await nextTick()
await clickMenu('项目的选项')
button('更改作用范围').props.onClick()
assert.deepEqual(managementAction, ['scope', 'local'])
await nextTick()
await clickMenu('项目的选项')
find(root, node => node.props.class === 'picker-list').props.onScroll()
await nextTick()
assert.equal(find(root, node => node.props.role === 'menu'), undefined, 'scrolling must close a menu anchored to its old position')

menuButton('全局主记忆的信息').props.onClick()
assert.equal(infoLibrary.id, 'global', 'main memory information must remain available')
await clickMenu('项目的选项')
menuButton('项目的信息').props.onClick()
await nextTick()
assert.equal(infoLibrary.id, 'local')
assert.equal(find(root, node => node.props.role === 'menu'), undefined, 'viewing information must close the operation menu')
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, ['local'], 'viewing information must not alter the enabled selection')
menuButton('研究笔记的信息').props.onClick()
assert.equal(infoLibrary.id, 'other-local', 'other workspaces must expose their information button too')
props.workspaces = workspaces.map(workspace => workspace.id === 'workspace-1' ? { ...workspace, name: '已改名项目' } : workspace)
await nextTick()
assert.ok(text(root).includes('已改名项目工作区记忆') && !text(root).includes('开发项目工作区记忆'), 'workspace renames must update labels without renaming the library')

// Refresh after a management action must preserve unsaved checkbox changes.
props.libraries = [...libraries, { id: 'extra', name: '新记忆', scope: 'global' }]
await nextTick()
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, ['local'])
props.libraries = [libraries[0]]
await nextTick()
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, [], 'deleted memory must disappear from unsaved selection too')
props.managing = true
await nextTick()
assert.equal(button('创建记忆').props.disabled, true)
assert.equal(menuButton('全局主记忆的信息').props.disabled, true)
button('取消').props.onClick()
assert.equal(cancelled, 0)
props.managing = false

props.saving = true
await nextTick()
assert.equal(button('正在保存…').props.disabled, true)
button('取消').props.onClick()
assert.equal(cancelled, 0)
props.saving = false
props.loadFailed = true
props.error = '加载失败'
await nextTick()
assert.equal(button('保存设置').props.disabled, true, 'failed loading must not allow clearing saved selection')
assert.ok(find(root, node => node.props.role === 'alert'))
props.open = false
await nextTick()
assert.equal(dialog.open, false)
app.unmount()

const formDescriptor = parse(fs.readFileSync(path.join(frontend, 'src/components/MemoryLibraryDialog.vue'), 'utf8')).descriptor
const formScript = compileScript(formDescriptor, { id: 'memory-library-form-test', inlineTemplate: true })
const { default: LibraryForm } = await import(dataModule(rewrite((await transformWithOxc(formScript.content, 'MemoryLibraryDialog.ts')).code)))
const formProps = reactive({ open: true, mode: 'create', workspaceId: 'workspace-1', workspaceName: '项目', busy: false, error: '' })
let formResult
const formApp = renderer.createApp({ render: () => h(LibraryForm, { ...formProps, onConfirm: (...args) => { formResult = args } }) })
formApp.mount(root)
await nextTick()
await nextTick()
const nameInput = find(body, node => node.type === 'input')
nameInput.value = '  项目笔记  '
nameInput.listeners.get('input')({ target: nameInput })
await nextTick()
const libraryForm = find(body, node => node.type === 'form')
libraryForm.props.onSubmit({ preventDefault() {} })
assert.deepEqual(formResult, ['项目笔记', 'workspace'], 'create form should use the current workspace by default')
formProps.open = false
await nextTick()
await nextTick()
formProps.workspaceId = ''
formProps.workspaceName = ''
formProps.open = true
await nextTick()
await nextTick()
assert.equal(find(body, node => node.type === 'option' && node.props.value === 'workspace').props.disabled, true)
const globalName = find(body, node => node.type === 'input')
globalName.value = '通用笔记'
globalName.listeners.get('input')({ target: globalName })
await nextTick()
find(body, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(formResult, ['通用笔记', 'global'])
formApp.unmount()

// Mount the real details component: native folds, local tag/content search, and load states.
const detailsDescriptor = parse(fs.readFileSync(path.join(frontend, 'src/components/MemoryDetailsDialog.vue'), 'utf8')).descriptor
const detailsScript = compileScript(detailsDescriptor, { id: 'memory-details-test', inlineTemplate: true })
const { default: DetailsDialog } = await import(dataModule(rewrite((await transformWithOxc(detailsScript.content, 'MemoryDetailsDialog.ts')).code)))
const detailsProps = reactive({ library: libraries[1], details: null, loading: true, error: '', workspaces, busy: false })
let detailsClosed = 0, detailsRetries = 0, entryAction
const detailsApp = renderer.createApp({ render: () => h(DetailsDialog, { ...detailsProps,
  onClose: () => { detailsClosed++ }, onRetry: () => { detailsRetries++ },
  onCreate: () => { entryAction = ['create'] }, onEdit: id => { entryAction = ['edit', id] }, onDelete: id => { entryAction = ['delete', id] },
}) })
detailsApp.mount(root)
await nextTick()
await nextTick()
const detailsDialog = find(body, node => node.type === 'dialog')
assert.equal(detailsDialog.open, true)
assert.ok(text(body).includes('正在加载记忆') && text(body).includes('项目') && text(body).includes('工作区记忆'))
assert.ok(text(body).includes('开发项目工作区记忆'), 'details should show the owning workspace while loading')
const snapshot = {
  ...libraries[1], description: '记录工作区的偏好',
  data: {
    'memory-a': { name: '开发偏好', data: '喜欢使用 Go 语言\n保留换行', date: '2026-11-18T06:00:00Z', description: ['开发', 'CAF'] },
    'memory-b': { name: '会议日程', data: '会议安排在星期三', date: '2026-11-18T07:00:00Z', description: ['日程', 'Meeting'] },
    'memory-c': { data: '咖啡不要加糖', date: '2026-11-18T08:00:00Z', description: null },
  },
}
detailsProps.details = snapshot
detailsProps.loading = false
await nextTick()
const entryNodes = () => {
  const rows = []
  function collect(node) {
    if (node.type === 'details' && node.props.class === 'memory-entry') rows.push(node)
    for (const child of node.children ?? []) collect(child)
  }
  collect(body)
  return rows
}
assert.equal(entryNodes().length, 3)
assert.ok(text(body).includes('开发项目工作区记忆'))
assert.ok(entryNodes().every(node => !node.open), 'individual memories should start folded')
assert.ok(text(entryNodes()[0]).includes('开发偏好'))
assert.equal(text(entryNodes()[0]).split(snapshot.data['memory-a'].data).length - 1, 1, 'memory content must be rendered exactly once')
assert.ok(!text(find(entryNodes()[0], node => node.type === 'summary')).includes(snapshot.data['memory-a'].data), 'folded titles should contain a name and tags, without repeating the body')
assert.ok(text(body).includes('记录工作区的偏好') && text(body).includes('memory-a') && text(body).includes('2026-11-18T06:00:00Z'))
const dataSection = find(body, node => node.type === 'details' && node.props.class === 'memory-data')
assert.equal(dataSection.open, true)
const searchInput = find(body, node => node.type === 'input' && node.props.type === 'search')
async function search(value) {
  searchInput.value = value
  searchInput.listeners.get('input')({ target: searchInput })
  await nextTick()
}
dataSection.open = false
await search('  caf  ')
assert.equal(entryNodes().length, 1, 'search should match tags without case or outer whitespace')
assert.ok(text(entryNodes()[0]).includes('喜欢使用 Go'))
assert.equal(dataSection.open, true, 'search should unfold the data section')
await search('星期三')
assert.equal(entryNodes().length, 1)
assert.ok(text(entryNodes()[0]).includes('会议安排'))
await search('咖啡')
assert.equal(entryNodes().length, 1, 'content search should handle null tags')
await search('gO')
assert.equal(entryNodes().length, 1, 'content matching should ignore case')
await search('找不到的内容')
assert.equal(entryNodes().length, 0)
assert.ok(text(body).includes('没有匹配的记忆') && text(body).includes('0 / 3'))
await search('')
assert.equal(entryNodes().length, 3)

find(body, node => node.type === 'button' && text(node) === '创建记忆').props.onClick()
assert.deepEqual(entryAction, ['create'])
const openEntryMenu = async label => {
  find(body, node => node.type === 'button' && node.props['aria-label'] === label).props.onClick({
    stopPropagation() {}, preventDefault() {}, currentTarget: { getBoundingClientRect: () => ({ top: 220, bottom: 248, right: 540 }) },
  })
  await nextTick()
}
await openEntryMenu('开发偏好的选项')
assert.equal(entryNodes()[0].open, false, 'opening the menu must not unfold the entry')
assert.equal(find(body, node => node.props.role === 'menu').parent, detailsDialog, 'entry menus must stay outside the scroll area')
find(body, node => node.type === 'button' && text(node) === '编辑').props.onClick()
await nextTick()
assert.deepEqual(entryAction, ['edit', 'memory-a'])
assert.equal(find(body, node => node.props.role === 'menu'), undefined)
await openEntryMenu('会议日程的选项')
find(body, node => node.type === 'button' && text(node) === '删除').props.onClick()
await nextTick()
assert.deepEqual(entryAction, ['delete', 'memory-b'])
await openEntryMenu('开发偏好的选项')
find(body, node => node.props.class === 'memory-details-content').props.onScroll()
await nextTick()
assert.equal(find(body, node => node.props.role === 'menu'), undefined)
await openEntryMenu('开发偏好的选项')
await search('CAF')
assert.equal(find(body, node => node.props.role === 'menu'), undefined, 'filtering must close the old menu')
await search('')
detailsProps.busy = true
await nextTick()
assert.equal(find(body, node => node.type === 'button' && text(node) === '创建记忆').props.disabled, true)
find(body, node => node.type === 'button' && text(node) === '关闭').props.onClick()
assert.equal(detailsClosed, 0, 'saving must keep the details dialog open')
detailsProps.busy = false

detailsProps.loading = true
await nextTick()
assert.equal(entryNodes().length, 0, 'loading should hide a previous snapshot')
detailsProps.loading = false
detailsProps.error = '正文加载失败'
await nextTick()
assert.equal(entryNodes().length, 0)
assert.ok(find(body, node => node.props.role === 'alert'))
find(body, node => node.type === 'button' && text(node) === '重新加载').props.onClick()
assert.equal(detailsRetries, 1)
detailsProps.error = ''
detailsProps.details = { ...libraries[0], data: {} }
detailsProps.library = libraries[0]
await nextTick()
await nextTick()
assert.ok(text(body).includes('全局主记忆') && text(body).includes('全局记忆') && text(body).includes('暂无记忆数据'))
await search('保留查询')
find(body, node => node.type === 'button' && text(node) === '关闭').props.onClick()
assert.equal(detailsClosed, 1)
detailsProps.library = null
await nextTick()
await nextTick()
assert.equal(detailsDialog.open, false)
detailsProps.library = libraries[1]
detailsProps.details = snapshot
await nextTick()
await nextTick()
assert.equal(detailsDialog.open, true)
assert.equal(find(body, node => node.type === 'input').value, '', 'reopening should clear the previous search')
assert.equal(entryNodes().length, 3)
detailsProps.library = libraries[2]
detailsProps.details = { ...libraries[2], data: {} }
await nextTick()
await nextTick()
assert.ok(text(body).includes('研究项目工作区记忆'), 'other-workspace details must use their own workspace name')
detailsProps.workspaces = [workspaces[0]]
await nextTick()
assert.ok(text(body).includes('已删除的工作区记忆'), 'a removed workspace must have a readable label')
detailsApp.unmount()

// Exercise the real create/edit form, including field preservation and tag normalization.
const entryDescriptor = parse(fs.readFileSync(path.join(frontend, 'src/components/MemoryEntryDialog.vue'), 'utf8')).descriptor
const entryScript = compileScript(entryDescriptor, { id: 'memory-entry-form-test', inlineTemplate: true })
const { default: EntryDialog } = await import(dataModule(rewrite((await transformWithOxc(entryScript.content, 'MemoryEntryDialog.ts')).code)))
const entryProps = reactive({ open: true, mode: 'edit', memoryId: 'memory-a', entry: snapshot.data['memory-a'], busy: false, error: '' })
let entryInput, entryCancelled = 0
const entryApp = renderer.createApp({ render: () => h(EntryDialog, { ...entryProps,
  onConfirm: input => { entryInput = input }, onCancel: () => { entryCancelled++ },
}) })
entryApp.mount(root)
await nextTick()
await nextTick()
const entryDialog = find(body, node => node.type === 'dialog')
assert.equal(entryDialog.open, true)
assert.ok(text(body).includes('编辑记忆') && text(body).includes('memory-a'))
const field = label => find(body, node => node.props['aria-label'] === label)
assert.equal(field('记忆名称').value, '开发偏好')
assert.equal(field('记忆内容').value, snapshot.data['memory-a'].data)
assert.equal(field('记忆时间').value, snapshot.data['memory-a'].date)
function setField(label, value) {
  const input = field(label)
  input.value = value
  input.listeners.get('input')({ target: input })
}
setField('记忆名称', '  新偏好  ')
setField('记忆时间', '2026-11-17T10:12:13.123456789+08:00')
setField('记忆内容', ' 新正文\n保持换行 ')
setField('记忆标签', ' Go, CAF\nGo，中文标签\n')
await nextTick()
find(body, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(entryInput, { name: '新偏好', date: '2026-11-17T10:12:13.123456789+08:00', data: ' 新正文\n保持换行 ', tags: ['Go', 'CAF', '中文标签'] })
entryInput = undefined
setField('记忆时间', '无效时间')
await nextTick()
find(body, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.equal(entryInput, undefined)
assert.equal(find(body, node => node.type === 'button' && text(node) === '确认保存').props.disabled, true)
entryProps.busy = true
await nextTick()
find(body, node => node.type === 'button' && text(node) === '取消').props.onClick()
assert.equal(entryCancelled, 0)
entryProps.busy = false
entryProps.error = '保存失败'
await nextTick()
assert.ok(find(body, node => node.props.role === 'alert'))
entryProps.open = false
await nextTick()
await nextTick()
assert.equal(entryDialog.open, false)
entryProps.mode = 'create'
entryProps.entry = undefined
entryProps.memoryId = undefined
entryProps.error = ''
entryProps.open = true
await nextTick()
await nextTick()
assert.ok(text(body).includes('创建记忆') && !text(body).includes('memory-a'))
assert.equal(field('记忆时间'), undefined, 'creation should use server time without a date field')
assert.equal(field('记忆名称').value, '')
setField('记忆名称', '新记忆')
setField('记忆内容', '记忆正文')
await nextTick()
find(body, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(entryInput, { name: '新记忆', data: '记忆正文', tags: [] })
entryApp.unmount()

const page = fs.readFileSync(path.join(frontend, 'src/pages/ChatPage.vue'), 'utf8')
assert.match(page, /role="menuitem"[^>]*@click="openMemorySettings"/)
assert.match(page, /<MemorySelectionDialog/)
console.log('Memory settings checks passed: floating menus, scoped labels, single body rendering, entry actions, create/edit forms, tags, local search, selection preservation, and retries.')
