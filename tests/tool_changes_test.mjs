import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'

const frontend = path.resolve('frontend')
const require = createRequire(path.join(frontend, 'package.json'))
const { transformWithOxc } = await import(pathToFileURL(require.resolve('vite')).href)
const { parse, compileScript } = await import(pathToFileURL(require.resolve('@vue/compiler-sfc')).href)
const { createRenderer, createSSRApp, h, nextTick } = await import(pathToFileURL(require.resolve('vue')).href)
const { renderToString } = await import(pathToFileURL(require.resolve('@vue/server-renderer')).href)
const { createPinia, setActivePinia } = await import(pathToFileURL(require.resolve('pinia')).href)
const dataModule = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const apiURL = dataModule((await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/api.ts'), 'utf8'), 'api.ts')).code)
const configsURL = dataModule('export const useConfigsStore = () => ({})')
function rewrite(code) {
  return code.replace(/from\s+(['"])([^'"]+)\1/g, (_whole, _quote, specifier) => {
    const url = specifier === '../api' ? apiURL : specifier === './configs' ? configsURL : pathToFileURL(require.resolve(specifier)).href
    return `from ${JSON.stringify(url)}`
  })
}
const storeCode = (await transformWithOxc(fs.readFileSync(path.join(frontend, 'src/stores/chat.ts'), 'utf8'), 'chat.ts')).code
const { useChatStore } = await import(dataModule(rewrite(storeCode)))
setActivePinia(createPinia())
const chat = useChatStore()
const { descriptor } = parse(fs.readFileSync(path.join(frontend, 'src/components/ToolChangesFooter.vue'), 'utf8'))
const script = compileScript(descriptor, { id: 'tool-changes-check', inlineTemplate: true })
const componentCode = (await transformWithOxc(script.content, 'ToolChangesFooter.ts')).code
const { default: ToolChangesFooter } = await import(dataModule(rewrite(componentCode)))

const operation = (id, action, state) => ({
  id, toolId: action === 'delete' ? 'DeleteFile' : 'WriteFile', callId: `call-${id}`, workspace: 'D:\\project\\workspace', state,
  changes: [{ target: `D:\\project\\workspace\\${id}.txt`, action, addedLines: action === 'delete' ? 0 : 2, removedLines: 1, beforeBytes: 10, afterBytes: 20 }],
})
const reply = (id, operations, next = []) => ({ id, choice: next.length ? 0 : -1, next, toolOperations: operations, child: [{ role: 'assistant', content: `${id} reply` }] })
const user = (id, next) => ({ id, choice: 0, next, child: [{ role: 'user', content: `${id} question` }] })
const original = {
  id: 'chat', tree: { root: { choice: 0, next: [user('first-user', [reply('first-reply', [operation('first', 'modify')], [
    user('second-user', [reply('second-reply', [operation('second', 'create'), operation('delete', 'delete')])]),
  ])])] } },
}
let saved = structuredClone(original)
let calls = [], complete, reject
globalThis.window = { go: { Backend: { Core: {
  ListChats: async () => [],
  SetChatToolChangesUndone: (...args) => {
    calls.push(args)
    return new Promise((resolve, fail) => { complete = resolve; reject = fail })
  },
} } } }
chat.session = structuredClone(saved)
chat.rebuildMessages()
const firstMessage = () => chat.messages.find(message => message.nodeId === 'first-reply')
const secondMessage = () => chat.messages.find(message => message.nodeId === 'second-reply')
assert.equal(firstMessage().canUndoToolChanges, false)
assert.equal(secondMessage().canUndoToolChanges, true)
assert.deepEqual(secondMessage().toolOperations, saved.tree.root.next[0].next[0].next[0].next[0].toolOperations)

function element(type) { return { type, text: '', props: {}, children: [], parent: null } }
function detach(node) { if (node.parent) node.parent.children.splice(node.parent.children.indexOf(node), 1) }
function find(node, predicate) { if (predicate(node)) return node; for (const child of node.children) { const found = find(child, predicate); if (found) return found } }
const nodeText = node => node.text + node.children.map(nodeText).join('')
const renderer = createRenderer({
  createElement: element, createText: text => ({ ...element('#text'), text }), createComment: () => element('#comment'),
  setText: (node, text) => { node.text = text }, setElementText: (node, text) => { node.text = text; node.children = [] },
  patchProp: (node, key, _previous, value) => { node.props[key] = value },
  insert(node, parent, anchor = null) { detach(node); node.parent = parent; parent.children.splice(anchor ? parent.children.indexOf(anchor) : parent.children.length, 0, node) },
  remove: detach, parentNode: node => node.parent, nextSibling: node => node.parent?.children[node.parent.children.indexOf(node) + 1] ?? null,
})
const root = element('root')
let submitted
const app = renderer.createApp({ render: () => {
  const message = secondMessage()
  return h(ToolChangesFooter, {
    operations: message.toolOperations, canUndo: message.canUndoToolChanges, canRedo: message.canRedoToolChanges,
    busy: chat.toolChangesChanging || chat.sending,
    onChange: undone => { submitted = chat.setToolChangesUndone(message, undone) },
  })
} })
app.mount(root)
const flush = async () => { await new Promise(resolve => setImmediate(resolve)); await nextTick() }
const button = () => find(root, node => node.type === 'button')
assert.match(nodeText(root), /本轮改动.*2 次操作.*撤销改动/)
assert.match(nodeText(root), /创建second\.txt.*\+2/)
assert.doesNotMatch(nodeText(root), /D:\\project/)
assert.equal(button().props.disabled, false)

button().props.onClick()
await flush()
assert.deepEqual(calls, [['chat', 'second-reply', true]])
assert.equal(button().props.disabled, true)
await chat.setToolChangesUndone(secondMessage(), true)
assert.equal(calls.length, 1, 'repeated click must not duplicate a file mutation')
reject(new Error('恢复缓存缺失'))
await submitted
await flush()
assert.match(chat.error, /恢复缓存缺失/)
assert.equal(button().props.disabled, false, 'failed undo must leave the action usable')
assert.equal(secondMessage().toolOperations[0].state, undefined)

function setSavedState(nodeId, state) {
  const node = nodeId === 'first-reply' ? saved.tree.root.next[0].next[0] : saved.tree.root.next[0].next[0].next[0].next[0]
  for (const entry of node.toolOperations) entry.state = state
}
button().props.onClick()
await flush()
setSavedState('second-reply', 'undone')
complete(structuredClone(saved))
await submitted
await flush()
assert.match(nodeText(root), /已撤销.*取消撤销/)
assert.equal(firstMessage().canUndoToolChanges, true)
assert.equal(secondMessage().canRedoToolChanges, true)
assert.equal(button().props.disabled, false)

let mutation = chat.setToolChangesUndone(firstMessage(), true)
await flush()
setSavedState('first-reply', 'undone')
complete(structuredClone(saved))
await mutation
await flush()
assert.equal(firstMessage().canRedoToolChanges, true)
assert.equal(secondMessage().canRedoToolChanges, false)
assert.equal(button().props.disabled, true)
const count = calls.length
await chat.setToolChangesUndone(secondMessage(), false)
assert.equal(calls.length, count, 'redo must follow execution order')

mutation = chat.setToolChangesUndone(firstMessage(), false)
await flush()
setSavedState('first-reply', undefined)
complete(structuredClone(saved))
await mutation
await flush()
button().props.onClick()
await flush()
assert.deepEqual(calls.at(-1), ['chat', 'second-reply', false])
setSavedState('second-reply', undefined)
complete(structuredClone(saved))
await submitted
await flush()
assert.match(nodeText(root), /撤销改动/)
assert.doesNotMatch(nodeText(root), /已撤销/)

chat.sending = true
await flush()
assert.equal(button().props.disabled, true)
await chat.setToolChangesUndone(secondMessage(), true)
assert.equal(calls.length, count + 2, 'generating chat must block undo')
chat.sending = false

const render = props => renderToString(createSSRApp({ render: () => h(ToolChangesFooter, props) }))
const binary = operation('binary', 'delete', 'discarded')
binary.changes[0].binary = true
let html = await render({ operations: [binary] })
assert.match(html, /10 → 20 字节/)
assert.match(html, /原改动保持撤销/)
assert.doesNotMatch(html, /取消撤销|<button/)
const escaped = operation('escape', 'modify')
escaped.changes[0].target = 'D:\\project\\workspace\\<script>name</script>.txt'
escaped.changes[0].approximate = true
html = await render({ operations: [escaped], canUndo: true })
assert.match(html, /&lt;script&gt;name&lt;\/script&gt;/)
assert.match(html, /change-estimate[^>]*>约/)
assert.doesNotMatch(html, /<script>/)

setSavedState('second-reply', undefined)
chat.session = structuredClone(saved)
chat.rebuildMessages()
mutation = chat.setToolChangesUndone(secondMessage(), true)
await flush()
chat.session = { id: 'other-chat', tree: { root: { choice: -1, next: [] } } }
complete(structuredClone(saved))
await mutation
assert.equal(chat.session.id, 'other-chat', 'an old undo reply must not replace another open conversation')
app.unmount()
console.log('Tool changes checks passed: per-response summaries, undo/redo ordering, retries, duplicate protection, safe rendering and conversation isolation.')
