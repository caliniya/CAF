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
  if (value === (props.config.Name || '')) return
  store.rename(props.index, value)
}
</script>

<template>
  <article class="card stack">
    <div class="row">
      <template v-if="renaming">
        <input
          ref="nameInput"
          v-model="draftName"
          type="text"
          placeholder="配置名称"
          @keydown.enter.prevent="submitRename"
          @keydown.esc.prevent="renaming = false"
        />
        <button type="button" class="btn-small" @click="submitRename">保存</button>
        <button
          type="button"
          class="btn-secondary btn-small"
          @click="renaming = false"
        >
          取消
        </button>
      </template>
      <template v-else>
        <h3>{{ config.Name || config.ServerAddress || '（未设置地址）' }}</h3>
        <span v-if="config.Name && config.ServerAddress" class="muted">
          {{ config.ServerAddress }}
        </span>
      </template>
      <span class="badge">#{{ index + 1 }}</span>
      <span v-if="enabled" class="badge badge-active">已启用</span>
    </div>

    <div class="field">
      <label>APIKey</label>
      <div class="row">
        <span class="notice mono">{{ keyDisplay }}</span>
        <button
          type="button"
          class="btn-tonal btn-small"
          @click="revealed = !revealed"
        >
          {{ revealed ? '隐藏' : '显示' }}
        </button>
      </div>
    </div>

    <div class="field">
      <label>模型</label>
      <div class="row">
        <template v-if="config.Model?.length">
          <span
            v-for="model in config.Model"
            :key="model.id"
            class="badge badge-clickable"
            role="button"
            tabindex="0"
            :class="{ 'badge-active': enabled && model.id === store.enabledModel }"
            :title="
              enabled && model.id === store.enabledModel ? modelTooltip(model) : model.name
            "
            @click="store.selectModel(index, model.id)"
            @keydown.enter.prevent="store.selectModel(index, model.id)"
            @keydown.space.prevent="store.selectModel(index, model.id)"
          >
            {{ model.name || model.id }}
          </span>
          <span v-if="enabled && !store.enabledModel" class="field-hint">
            尚未选择模型，点击模型即可启用。
          </span>
        </template>
        <span v-else class="muted">未配置模型</span>
      </div>
    </div>

    <div class="row">
      <button
        v-if="!enabled"
        type="button"
        class="btn-tonal btn-small"
        @click="store.selectConfig(index)"
      >
        启用此配置
      </button>
      <button type="button" class="btn-secondary btn-small" @click="beginRename">
        重命名
      </button>
      <button
        type="button"
        class="btn-secondary btn-small"
        :disabled="refreshing"
        @click="refreshModels"
      >
        {{ refreshing ? '正在探测…' : '重新探测' }}
      </button>
    </div>
    <p v-if="refreshMessage" class="muted" role="status">{{ refreshMessage }}</p>
    <ModelSelectionDialog
      :open="selectionOpen"
      :models="discoveredModels"
      confirm-label="保存选中模型"
      @confirm="saveRefreshedModels"
      @cancel="selectionOpen = false"
    />
  </article>
</template>
