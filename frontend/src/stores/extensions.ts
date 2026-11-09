import { ref } from 'vue'
import { defineStore } from 'pinia'
import { CreateExtensionClient, DeleteExtensionClient, ListExtensionClients, RenameExtensionClient,
  RemoveExtensionClientQQ, SetExtensionClientQQ, ListExtensionTypes, ListExtensions, CreateExtension,
  UpdateExtensionParameters, SetClientExtensionEnabled, DeleteExtension,
  type ExtensionClient, type ExtensionDescriptor, type ExtensionOption } from '../api'

export const useExtensionStore = defineStore('extensions', () => {
  const clients = ref<ExtensionClient[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const pending = ref<string[]>([])
  const types = ref<ExtensionDescriptor[]>([])
  const options = ref<ExtensionOption[]>([])
  const optionsLoaded = ref(false)
  const optionsError = ref('')
  let polling: ReturnType<typeof setInterval> | undefined
  let watchers = 0
  let optionsRefreshing = false
  let revision = 0
  async function refresh() {
    if (loading.value) return
    loading.value = true
    error.value = ''
    const before = revision
    try {
      const result = await ListExtensionClients()
      if (before === revision) clients.value = result
      loaded.value = true
    } catch (err) { error.value = String(err) }
    finally { loading.value = false }
  }
  function replace(client: ExtensionClient) {
    const index = clients.value.findIndex((item) => item.id === client.id)
    if (index < 0) clients.value.push(client)
    else clients.value[index] = client
  }
  async function mutate<T>(id: string, action: () => Promise<T>) {
    if (pending.value.includes(id)) throw new Error('操作正在进行中')
    pending.value.push(id); revision++
    try { return await action() }
    finally { pending.value = pending.value.filter((item) => item !== id); revision++ }
  }
  async function create(name: string, id: string) {
    return mutate('create', async () => { const client = await CreateExtensionClient(name, id); replace(client); return client })
  }
  async function rename(id: string, name: string) {
    return mutate(id, async () => { const client = await RenameExtensionClient(id, name); replace(client); return client })
  }
  async function setQQ(id: string, adapterId: string, name: string, enabled: boolean) {
    return mutate(id, async () => { const client = await SetExtensionClientQQ(id, adapterId, name, enabled); replace(client); return client })
  }
  async function removeQQ(id: string, adapterId: string) {
    return mutate(id, async () => { const client = await RemoveExtensionClientQQ(id, adapterId); replace(client); return client })
  }
  async function remove(id: string) {
    return mutate(id, async () => { await DeleteExtensionClient(id); clients.value = clients.value.filter((item) => item.id !== id) })
  }
  async function refreshOptions() {
    if (optionsRefreshing) return
    optionsRefreshing = true
    const before = revision
    try {
      const [descriptors, result] = await Promise.all([ListExtensionTypes(), ListExtensions()])
      if (before === revision && !pending.value.length) { types.value = descriptors; options.value = result; optionsLoaded.value = true; optionsError.value = '' }
    } catch (err) { optionsError.value = String(err) }
    finally { optionsRefreshing = false }
  }
  function watchOptions() {
    watchers++
    void refreshOptions()
    polling ??= setInterval(() => void refreshOptions(), 1000)
    let stopped = false
    return () => { if (stopped) return; stopped = true; if (--watchers === 0) { clearInterval(polling); polling = undefined } }
  }
  async function createOption(type: string, parameters: Record<string, string>) {
    const result = await mutate('new-extension', () => CreateExtension(type, parameters))
    await refreshOptions()
    return result
  }
  async function updateOption(id: string, parameters: Record<string, string>) {
    await mutate('extension:' + id, () => UpdateExtensionParameters(id, parameters))
    await Promise.all([refreshOptions(), refresh()])
  }
  async function enableOption(clientId: string, id: string, enabled: boolean) {
    await mutate('extension:' + id, async () => replace(await SetClientExtensionEnabled(clientId, id, enabled)))
    await refreshOptions()
  }
  async function deleteOption(id: string) {
    await mutate('extension:' + id, () => DeleteExtension(id))
    options.value = options.value.filter((item) => item.id !== id)
  }
  async function detachOption(clientId: string, id: string) {
    await mutate('extension:' + id, async () => replace(await RemoveExtensionClientQQ(clientId, id)))
    await refreshOptions()
  }
  return { clients, loading, loaded, error, pending, refresh, replace, create, rename, setQQ, removeQQ, remove,
    types, options, optionsLoaded, optionsError, refreshOptions, watchOptions, createOption, updateOption, enableOption, deleteOption, detachOption }
})
