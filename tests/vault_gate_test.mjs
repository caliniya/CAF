import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'

const frontend = new URL('../frontend/', import.meta.url)
const require = createRequire(new URL('package.json', frontend))
const packageURL = name => pathToFileURL(require.resolve(name)).href
const { transformWithOxc } = await import(packageURL('vite'))
const { parse, compileScript } = await import(packageURL('@vue/compiler-sfc'))
const { createRenderer, nextTick } = await import(packageURL('vue'))
const moduleURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const read = file => readFileSync(new URL(file, frontend), 'utf8')
const api = moduleURL((await transformWithOxc(read('src/api.ts'), 'api.ts')).code)
async function compiledURL(file, extra = {}) {
  const { descriptor } = parse(read(file))
  const script = compileScript(descriptor, { id: 'profile-test', inlineTemplate: true })
  const { code } = await transformWithOxc(script.content, 'ProfileComponent.ts')
  const deps = { vue: packageURL('vue'), '../api': api, ...extra }
  return moduleURL(code.replace(/from\s+(['"])([^'"]+)\1/g, (_, q, specifier) => `from ${JSON.stringify(deps[specifier])}`))
}
const Gate = (await import(await compiledURL('src/components/VaultGate.vue'))).default

function node(tag, text = '') {
  const events = new Map()
  return { tag, text, children: [], props: {}, value: '', getRootNode: () => ({}),
    addEventListener: (name, fn) => events.set(name, fn), fire(name) { events.get(name)({ target: this }) } }
}
globalThis.Document = class {}
globalThis.ShadowRoot = class {}
function remove(el) { if (el.parent) el.parent.children.splice(el.parent.children.indexOf(el), 1); el.parent = null }
const renderer = createRenderer({
  createElement: node, createText: text => node('#text', text), createComment: () => node('#comment'),
  patchProp: (el, prop, _, value) => { el.props[prop] = value },
  setText: (el, text) => { el.text = text }, setElementText: (el, text) => { el.text = text; el.children = [] },
  insert(el, parent, anchor = null) { remove(el); el.parent = parent; parent.children.splice(anchor ? parent.children.indexOf(anchor) : parent.children.length, 0, el) },
  remove, parentNode: el => el.parent, nextSibling: el => el.parent?.children[el.parent.children.indexOf(el) + 1] ?? null,
})
function* walk(el) { yield el; for (const child of el.children) yield* walk(child) }
const flush = async () => { await new Promise(resolve => setImmediate(resolve)); await nextTick() }
async function mount(api, onReady, view = Gate) {
  globalThis.window = { go: { Backend: { Accounts: api } } }
  const root = node('root'), app = renderer.createApp(view, { onReady })
  app.mount(root)
  await flush()
  const find = tag => [...walk(root)].find(n => n.tag === tag)
  return { find, nodes: () => [...walk(root)], button: text => [...walk(root)].find(n => n.tag === 'button' && n.text === text), close: () => app.unmount(), inputs: () => [...walk(root)].filter(n => n.tag === 'input'),
    submit: async () => { await find('form').props.onSubmit({ preventDefault() {} }); await flush() },
    proceed: async () => { await find('button').props.onClick(); await flush() },
    alert: () => [...walk(root)].find(n => n.props.role === 'alert')?.text }
}
const fill = (input, value) => { input.value = value; input.fire('input') }

// A device-store failure must leave an unlocked vault usable, without re-creating it.
let unlocked = false, ready = 0, starts = 0
const pass = '🌸乙丙丁'
const first = await mount({
  GetVaultStatus: async () => ({ configured: unlocked, unlocked, remembered: false }),
  CreateVault: async value => { assert.equal(value, pass); assert.equal(unlocked, false); unlocked = true },
  SetDefaultProfile: async (_ticket, on) => { assert.equal(on, true); throw Error('no system keychain') },
  StartVaultSession: async () => { if (++starts === 1) throw Error('migration pending') },
}, () => ++ready)
const [master, repeat, remember] = first.inputs()
fill(master, '🌸乙丙'); fill(repeat, '🌸乙丙'); await nextTick()
assert.equal(first.find('button').props.disabled, true)
fill(master, pass); fill(repeat, 'mistyped')
await nextTick()
assert.equal(first.find('button').props.disabled, true)
await first.submit()
assert.equal(unlocked, false)
fill(repeat, pass); remember.checked = true; remember.fire('change')
await first.submit()
assert.match(first.alert(), /系统未能记住/)
assert.equal(first.inputs().length, 0)
assert.equal(ready, 0)
await first.proceed()
assert.match(first.alert(), /migration pending/)
assert.equal(ready, 0, 'chat must stay gated after a migration failure')
await first.proceed()
assert.equal(ready, 1)
first.close()

// On a copied directory, unlocking has no side effect on the new device's keychain by default.
unlocked = false; ready = 0
const copied = await mount({
  GetVaultStatus: async () => ({ configured: true, unlocked, remembered: false }),
  UnlockVault: async value => { if (value !== pass) throw Error('主密码不正确'); unlocked = true },
  StartVaultSession: async () => assert.equal(unlocked, true),
}, () => ++ready)
assert.equal(copied.inputs().length, 2)
fill(copied.inputs()[0], 'incorrect')
await copied.submit()
assert.match(copied.alert(), /主密码不正确/)
assert.equal(ready, 0)
fill(copied.inputs()[0], pass)
await copied.submit()
assert.equal(ready, 1)
copied.close()
const remembered = await mount({ GetVaultStatus: async () => ({ configured: true, unlocked: true, remembered: true }),
  StartVaultSession: async () => {} }, () => ++ready)
assert.equal(remembered.inputs().length, 0)
assert.equal(ready, 2)
remembered.close()

// Registration is no longer available to a signed-out visitor.
const existing = await mount({ GetVaultStatus: async () => ({ configured: true, unlocked: false, remembered: false }),
  UnlockVault: async () => {}, StartVaultSession: async () => 'active-ticket' })
assert.equal(existing.button('创建新用户'), undefined)
assert.equal(existing.inputs().length, 2)
fill(existing.inputs()[0], pass); await existing.submit(); existing.close()

// Render both real settings components. The native dialog and directory picker are substituted here.
const confirmStub = moduleURL(`import {h} from '${packageURL('vue')}'; export default {props:['open'],emits:['confirm','cancel'],setup(p,{emit}){ return () => p.open ? h('button',{onClick:()=>emit('confirm')},'确认退出') : null }}`)
const windowStub = moduleURL(`import {h} from '${packageURL('vue')}'; export default {props:['open','title','busy'],emits:['close'],setup(p,{slots}){return ()=>p.open?h('dialog',{role:'dialog'},[h('h2',p.title),slots.default?.(),slots.footer?.()]):null}}`)
const members = await compiledURL('src/components/InternalUsersSettings.vue', { './ExtensionWindow.vue': windowStub })
const Settings = (await import(await compiledURL('src/pages/SettingsPlatformPage.vue', { '../components/ConfirmationDialog.vue': confirmStub,
  '../components/ExtensionWindow.vue': windowStub, '../components/InternalUsersSettings.vue': members }))).default
const calls = []; let reloads = 0, picker = 'error', role = 'owner', listed = 0
const accounts = {
  GetVaultStatus: async () => ({configured:true, unlocked:true, remembered:false, role}),
  ListInternalUsers: async () => { ++listed; return [] },
  Call: async (ticket, method) => { assert.equal(ticket,'active-ticket'); assert.equal(method,'LoadConfigs'); return {config:[
    {ID:'p1', Name:'One', Model:[{id:'m'}]}, {ID:'p2', Name:'Two', Model:[{id:'m'}]}]} },
  CreateInternalUser: async (ticket, pwd, models) => { calls.push(['create',ticket,pwd,...models]); return {id:'internal-id',models} },
  SetInternalUserModels: async (ticket,id,models) => { calls.push(['grant',ticket,id,...models]); return {id,models} },
  ChangePassword: async (...args) => calls.push(['change',...args]), SetDefaultProfile: async (...args) => calls.push(['default',...args]),
  ExportUserData: async (...args) => { calls.push(['export',...args]); if (picker==='error') throw Error('密码不正确'); return picker },
  Logout: async (...args) => calls.push(['logout',...args]),
}
const settings = await mount(accounts, undefined, Settings)
const click = async (view, text) => { const b = view.button(text); assert.ok(b, text); await b.props.onClick(); await flush() }
const inDialog = view => [...walk(view.find('dialog'))]
const dialogInputs = view => inDialog(view).filter(n=>n.tag==='input')
const checked = (input,on) => { input.checked=on; input.fire('change') }
window.location = { hash:'', reload:()=>++reloads }
await settings.inputs()[0].props.onClick({preventDefault(){}});await flush()
assert.deepEqual(calls.shift(), ['default','active-ticket',true])
await click(settings,'更改密码')
let [currentKey,newKey,confirmKey] = dialogInputs(settings);fill(currentKey,pass);fill(newKey,'甲乙丙丁');fill(confirmKey,'丙丁');await nextTick()
assert.equal(settings.button('保存新密码').props.disabled, true)
fill(confirmKey,'甲乙丙丁');await settings.submit();assert.deepEqual(calls.shift(), ['change','active-ticket',pass,'甲乙丙丁'])
await click(settings,'导出原始数据')
assert.equal(settings.button('选择目录并导出').props.disabled,true)
assert.equal(settings.button('选择目录并导出').props.form, settings.find('form').props.id)
fill(dialogInputs(settings)[0],pass);await settings.submit();assert.deepEqual(calls.shift(),['export','active-ticket',pass,false])
assert.ok(inDialog(settings).some(n=>n.props.role==='alert' && n.text.includes('密码不正确')),'the export error belongs inside the open dialog')
picker='';await settings.submit();assert.ok(inDialog(settings).some(n=>n.props.role==='status' && n.text.includes('未选择目录')))
assert.deepEqual(calls.shift(),['export','active-ticket',pass,false]);checked(dialogInputs(settings)[1],true)
picker='user.zip';await settings.submit();assert.deepEqual(calls.shift(),['export','active-ticket',pass,true])
assert.equal(settings.find('dialog'),undefined);assert.ok(settings.nodes().some(n=>n.props.role==='status' && n.text.includes('user.zip')))
await click(settings,'导出原始数据');assert.equal(settings.button('选择目录并导出').props.disabled,true);assert.ok(!dialogInputs(settings)[1].checked);await click(settings,'取消')
await click(settings,'创建内部用户');const [guestPass,confirmGuest,,modelTwo]=dialogInputs(settings)
fill(guestPass,'123');fill(confirmGuest,'123');await nextTick();assert.equal(settings.button('确认创建').props.disabled,true)
fill(guestPass,'1234');fill(confirmGuest,'1234');modelTwo.props.onChange();await settings.submit()
assert.deepEqual(calls.shift(),['create','active-ticket','1234',{configId:'p2',modelId:'m'}]);assert.equal(settings.find('dialog'),undefined)
await click(settings,'模型权限');const [one,two]=dialogInputs(settings);assert.equal(one.props.checked,false);assert.equal(two.props.checked,true)
two.props.onChange();await settings.submit();assert.deepEqual(calls.shift(),['grant','active-ticket','internal-id'])
await click(settings,'退出登录');assert.equal(calls.length,0)
await click(settings,'确认退出');assert.deepEqual(calls.shift(), ['logout','active-ticket']);assert.equal(reloads,1);settings.close()
role='internal';const guest=await mount(accounts,undefined,Settings)
assert.equal(listed,1,'guest must never request the management list');assert.equal(guest.button('创建内部用户'),undefined)
await click(guest,'导出原始数据');assert.ok(inDialog(guest).some(n=>n.text.includes('自己的明文 QQ Token')));assert.ok(!inDialog(guest).some(n=>n.text.includes('API Key')));guest.close()
