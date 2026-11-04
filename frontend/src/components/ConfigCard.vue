<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useConfigsStore } from '../stores/configs'
import ModelSelectionDialog from './ModelSelectionDialog.vue'
import type { Config, Model } from '../api'

const props = defineProps<{ config: Config; index: number }>()
const store = useConfigsStore()

// APIKey 默认打码，按卡片独立切换显示。
const revealed = ref(false)
const keyDisplay = computed(() => {
  const key = props.config.APIKey
  if (revealed.value) return key || '（未设置）'
  if (!key) return '（未设置）'
  if (key.length <= 10) return `${key.slice(0, 2)}…`
  return `${key.slice(0, 6)}…${key.slice(-4)}`
})

const enabled = computed(() => store.isEnabled(props.config))
const refreshing = ref(false)
const refreshMessage = ref('')
const selectionOpen = ref(false)
const discoveredModels = ref<Model[]>([])
const probedServerAddress = ref('')

async function refreshModels() {
  refreshing.value = true
  refreshMessage.value = ''
  try {
    const result = await store.probeModels(props.config.ServerAddress, props.config.APIKey)
    discoveredModels.value = Array.isArray(result?.models) ? result.models : []
    probedServerAddress.value = result?.serverAddress || props.config.ServerAddress
    selectionOpen.value = true
  } catch (error) {
    refreshMessage.value = `重新探测失败：${String(error)}`
  } finally {
    refreshing.value = false
  }
}

async function saveRefreshedModels(selected: Model[]) {
  selectionOpen.value = false
  refreshing.value = true
  refreshMessage.value = ''
  try {
    await store.refreshModelConfig(props.index, probedServerAddress.value, selected)
    refreshMessage.value = `已保存 ${selected.length} 个模型。`
  } catch (error) {
    refreshMessage.value = `保存探测结果失败：${String(error)}`
  } finally {
    refreshing.value = false
  }
}

function confirmDelete() {
  const label = props.config.Name || props.config.ServerAddress
  if (!window.confirm(`确定删除配置「${label}」吗？此操作不可撤销。`)) return
  store.deleteConfig(props.index)
}

function modelTooltip(model: Config['Model'][number]) {
  const details = [`模型 ID: ${model.id}`]
  if (model.context_window) details.push(`上下文: ${model.context_window.toLocaleString()} tokens`)
  if (model.max_output_tokens) details.push(`最大输出: ${model.max_output_tokens.toLocaleString()} tokens`)
  return details.join(' · ')
}

// 行内重命名编辑器的局部状态。
const renaming = ref(false)
const draftName = ref('')
const nameInput = ref<HTMLInputElement | null>(null)

function beginRename() {
  draftName.value = props.config.Name || ''
  renaming.value = true
  nextTick(() => {
    nameInput.value?.focus()
    nameInput.value?.select()
  })
}

function submitRename() {
  renaming.value = false
  const value = draftName.value.trim()
  // 名称未变化时不触发保存。
  if (value === (props.config.Name || '')) return
  store.rename(props.index, value)
}
</script>

<template>
  <div data-placeholder="ConfigCard"></div>
</template>
