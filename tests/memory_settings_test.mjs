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
const configsURL = dataModule('export const useConfigsStore = () => ({ enables: { ID: "provider" }, enabledModel: "model" })')
function rewrite(code) {
  return code.replace(/from\s+(['"])([^'"]+)\1/g, (_whole, _quote, specifier) => {
    const url = specifier === '../api' ? apiURL : specifier === './configs' ? configsURL : pathToFileURL(require.resolve(specifier)).href
    return `from ${JSON.stringify(url)}`
  })
}
const storeURL = dataModule(rewrite((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/stores/chat.ts'), 'utf8'), 'chat.ts')).code))
const { useChatStore } = await import(storeURL)
setActivePinia(createPinia())
const chat = useChatStore()
let resolveSave, resolveLoad
let savedIDs = []
const libraries = [{ id: 'global', name: 'Main', description: '全局记忆库', scope: 'global' }, { id: 'local', name: '项目', scope: 'workspace' }]
globalThis.window = { go: { Backend: { Core: {
  GetChatMemorySettings: async () => ({ libraries, enabledIds: ['global'] }),
  SetChatMemoryLibraries: (id, ids) => {
    savedIDs = [...ids]
    return new Promise(resolve => { resolveSave = () => resolve({ id, memoryLibraryIds: ids }) })
  },
  CreateChat: async () => ({ id: 'new-chat', tree: { root: null } }),
  ListChats: async () => [],
} } } }

chat.session = { id: 'chat-1', memoryLibraryIds: ['missing'] }
await chat.loadMemorySettings()
assert.deepEqual(chat.enabledMemoryLibraryIds, ['global'])
const saving = chat.setMemoryLibraries(['local'])
await nextTick()
await Promise.resolve()
chat.session = { id: 'chat-2', memoryLibraryIds: [] }
resolveSave()
await saving
assert.deepEqual(chat.enabledMemoryLibraryIds, [], 'save response for an older chat must not overwrite new chat')

window.go.Backend.Core.GetChatMemorySettings = () => new Promise(resolve => { resolveLoad = resolve })
const loading = chat.loadMemorySettings()
await Promise.resolve()
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
assert.deepEqual(chat.pendingMemoryLibraryIds, [])

function element(type) {
  return { type, tagName: type.toUpperCase(), props: {}, children: [], parent: null, listeners: new Map(), checked: false, open: false,
    addEventListener(event, callback) { this.listeners.set(event, callback) },
    removeEventListener() {},
    showModal() { this.open = true }, close() { this.open = false },
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
function text(node) { return (node.text ?? '') + (node.children ?? []).map(text).join('') }
const renderer = createRenderer({
  createElement: element, createText: value => ({ ...element('#text'), text: value }), createComment: value => ({ ...element('#comment'), text: value }),
  setText: (node, value) => { node.text = value }, setElementText: (node, value) => { node.text = value; node.children = [] },
  patchProp: (node, key, _old, value) => { node.props[key] = value; if (key === 'value') node._value = value },
  insert(node, parent, anchor = null) { detach(node); node.parent = parent; parent.children.splice(anchor ? parent.children.indexOf(anchor) : parent.children.length, 0, node) },
  remove: detach, parentNode: node => node.parent,
  nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
})
const { descriptor } = parse(fs.readFileSync(path.join(frontend, 'src/components/MemorySelectionDialog.vue'), 'utf8'))
const script = compileScript(descriptor, { id: 'memory-dialog-test', inlineTemplate: true })
const { default: Dialog } = await import(dataModule(rewrite((await transformWithOxc(script.content, 'MemorySelectionDialog.ts')).code)))
const props = reactive({ open: true, libraries, enabledIds: ['global', 'missing'], loading: false, saving: false, loadFailed: false, error: '' })
const root = element('root')
let confirmed, cancelled = 0
const app = renderer.createApp({ render: () => h(Dialog, { ...props, onConfirm: ids => { confirmed = ids }, onCancel: () => { cancelled++ } }) })
app.mount(root)
await nextTick()
await nextTick()
const button = label => find(root, node => node.type === 'button' && text(node) === label)
const dialog = find(root, node => node.type === 'dialog')
assert.equal(dialog.open, true)
assert.ok(text(root).includes('记忆设置') && text(root).includes('全局') && text(root).includes('工作区'))
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, ['global'], 'missing saved ID must not remain selected')

button('全选').props.onClick()
await nextTick()
find(root, node => node.type === 'form').props.onSubmit({ preventDefault() {} })
assert.deepEqual(confirmed, ['global', 'local'])
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
const page = fs.readFileSync(path.join(frontend, 'src/pages/ChatPage.vue'), 'utf8')
assert.match(page, /role="menuitem"[^>]*@click="openMemorySettings"/)
assert.match(page, /<MemorySelectionDialog/)
console.log('Memory settings checks passed: selection, disable all, draft persistence, loading errors, busy state, and chat-switch isolation.')
