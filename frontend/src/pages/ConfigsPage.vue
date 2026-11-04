<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useConfigsStore } from '../stores/configs'
import ConfigCard from '../components/ConfigCard.vue'
import CurrentPanel from '../components/CurrentPanel.vue'
import ModelSelectionDialog from '../components/ModelSelectionDialog.vue'
import type { Model } from '../api'

const store = useConfigsStore()
const adding = ref(false)
const probing = ref(false)
const probeMessage = ref('')
const name = ref('')
const serverAddress = ref('')
const protocol = ref<'chat-completions'>('chat-completions')
const probedServerAddress = ref('')
const apiKey = ref('')
const discoveredModels = ref<Model[]>([])
const selectedModels = ref<string[]>([])
const selectionOpen = ref(false)
const saving = ref(false)
onMounted(() => {
  if (!store.loaded) store.load()
})

watch([serverAddress, apiKey], ([address, key]) => {
  probedServerAddress.value = ''
  discoveredModels.value = []
  selectedModels.value = []
  if (!address.trim() || !key.trim()) probeMessage.value = ''
})

function resetForm() {
  name.value = ''
  serverAddress.value = ''
  protocol.value = 'chat-completions'
  probedServerAddress.value = ''
  apiKey.value = ''
  discoveredModels.value = []
  selectedModels.value = []
  selectionOpen.value = false
  probeMessage.value = ''
}

async function probe() {
  if (!serverAddress.value.trim()) {
    probeMessage.value = '请先填写 API 地址。'
    return
  }
  probing.value = true
  probeMessage.value = ''
  discoveredModels.value = []
  selectedModels.value = []
  try {
    const result = await store.probeModels(serverAddress.value, apiKey.value)
    const models = Array.isArray(result?.models) ? result.models : []
    probedServerAddress.value = result.serverAddress
    discoveredModels.value = models
    if (!models.length) {
      probeMessage.value = '接口没有返回模型，可在选择窗口中手动添加模型 ID。'
    }
    selectionOpen.value = true
  } catch (error) {
    probeMessage.value = `自动探测失败：${String(error)}。可以手动添加模型 ID。`
    discoveredModels.value = []
    selectionOpen.value = true
  } finally {
    probing.value = false
  }
}

function acceptModelSelection(selected: Model[], available: Model[]) {
  discoveredModels.value = available
  selectedModels.value = selected.map((model) => model.id)
  selectionOpen.value = false
  probeMessage.value = `已选择 ${selected.length} 个模型。`
}

async function saveConfig() {
  const models = discoveredModels.value.filter((model) => selectedModels.value.includes(model.id))
  if (!models.length) {
    probeMessage.value = '请先探测并选择至少一个模型。'
    return
  }
  saving.value = true
  try {
    await store.addModelConfig(name.value, probedServerAddress.value || serverAddress.value, protocol.value, apiKey.value, models)
    if (!store.error) {
      adding.value = false
      resetForm()
    }
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <section class="page-scroll" data-placeholder="ConfigsPage"></section>
</template>
