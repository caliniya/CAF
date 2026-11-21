import { computed, onMounted, onUnmounted, ref, watch, type Ref } from 'vue'
import { GetExtensionClient } from '../api'
import { useExtensionStore } from '../stores/extensions'

// 详情页独立加载客户端，轮询继续更新扩展状态；旧路由的响应不能覆盖新页面。
export function useExtensionClientPage(clientId: Readonly<Ref<string>>) {
  const ext = useExtensionStore()
  const client = computed(() => ext.clients.find((value) => value.id === clientId.value))
  const loading = ref(true)
  const error = ref('')
  let generation = 0
  let stop: (() => void) | undefined

  async function load() {
    const current = ++generation
    loading.value = true
    error.value = ''
    try {
      const [result] = await Promise.all([GetExtensionClient(clientId.value), ext.refreshOptions()])
      if (current === generation) ext.replace(result)
    } catch (err) {
      if (current === generation) error.value = String(err)
    } finally {
      if (current === generation) loading.value = false
    }
  }

  watch(clientId, () => void load(), { immediate: true })
  onMounted(() => { stop = ext.watchOptions() })
  onUnmounted(() => { generation++; stop?.() })
  return { ext, client, loading, error, load }
}
