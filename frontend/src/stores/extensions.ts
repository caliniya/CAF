import { ref } from 'vue'
import { defineStore } from 'pinia'
import { CreateExtensionClient, DeleteExtensionClient, ListExtensionClients, RenameExtensionClient,
  RemoveExtensionClientQQ, SetExtensionClientQQ, type ExtensionClient } from '../api'

export const useExtensionStore = defineStore('extensions', () => {
  const clients = ref<ExtensionClient[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const pending = ref<string[]>([])
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
  return { clients, loading, loaded, error, pending, refresh, replace, create, rename, setQQ, removeQQ, remove }
})
