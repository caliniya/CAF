import { ref } from 'vue'
import { defineStore } from 'pinia'
import {
  CreateQQAdapter, DeleteQQAdapter, ListQQAdapters, StartQQAdapter,
  StopQQAdapter, UpdateQQAdapter, type QQAdapter, type QQAdapterInput,
} from '../api'

export const QQ_STATE_LABELS = {
  stopped: '未启动',
  disconnected: '未连接',
  connected: '已连接',
} as const

export const useQQStore = defineStore('qq', () => {
  const adapters = ref<QQAdapter[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref('')
  const pending = ref<string[]>([])
  let refreshing = false
  let revision = 0
  let subscribers = 0
  let timer: ReturnType<typeof setInterval> | undefined

  async function refresh(silent = false) {
    if (refreshing) return
    refreshing = true
    if (!silent) { loading.value = true; error.value = '' }
    const currentRevision = revision
    try {
      const result = await ListQQAdapters()
      if (revision === currentRevision && !pending.value.length) {
        adapters.value = result
        loaded.value = true
      }
    } catch (err) {
      error.value = String(err)
    } finally {
      refreshing = false
      loading.value = false
    }
  }

  function watchStatus() {
    subscribers++
    void refresh()
    timer ??= setInterval(() => void refresh(true), 1000)
    let stopped = false
    return () => {
      if (stopped) return
      stopped = true
      if (--subscribers === 0) { clearInterval(timer); timer = undefined }
    }
  }

  function replace(adapter: QQAdapter) {
    const index = adapters.value.findIndex((item) => item.id === adapter.id)
    if (index < 0) adapters.value.push(adapter)
    else adapters.value[index] = adapter
  }

  async function mutate<T>(id: string, action: () => Promise<T>): Promise<T> {
    if (pending.value.includes(id)) throw new Error('操作正在进行中')
    pending.value.push(id)
    revision++
    try { return await action() }
    finally {
      pending.value = pending.value.filter((item) => item !== id)
      revision++
    }
  }

  async function create(input: QQAdapterInput) {
    return mutate('create', async () => {
      const adapter = await CreateQQAdapter(input)
      replace(adapter)
      return adapter
    })
  }

  async function update(id: string, input: QQAdapterInput) {
    return mutate(id, async () => {
      const adapter = await UpdateQQAdapter(id, input)
      replace(adapter)
      return adapter
    })
  }

  async function toggle(adapter: QQAdapter) {
    error.value = ''
    try {
      await mutate(adapter.id, async () => {
        replace(await (adapter.state === 'stopped' ? StartQQAdapter(adapter.id) : StopQQAdapter(adapter.id)))
      })
    } catch (err) { error.value = String(err) }
  }

  async function remove(id: string) {
    await mutate(id, async () => {
      await DeleteQQAdapter(id)
      adapters.value = adapters.value.filter((item) => item.id !== id)
    })
  }

  return { adapters, loading, loaded, error, pending, refresh, watchStatus, create, update, toggle, remove }
})
