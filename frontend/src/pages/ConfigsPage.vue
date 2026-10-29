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
  <section class="card stack page-scroll">
    <p class="description">
      读取数据目录下 config/MainConfig.json 中的模型服务配置；点击「启用」选择提供商，点击模型徽章选定要使用的模型。
      <span v-if="store.configs.length" class="badge">{{ store.configs.length }} 项</span>
    </p>

    <CurrentPanel v-if="store.loaded" />

    <div class="row">
      <button type="button" class="btn" @click="adding = !adding; resetForm()">
        {{ adding ? '取消添加' : '＋ 添加模型服务' }}
      </button>
      <button type="button" :disabled="store.loading" @click="store.load()">
        重新加载
      </button>
    </div>

    <form v-if="adding" class="stack model-add-form" @submit.prevent="saveConfig">
      <h3>添加模型服务</h3>
      <label class="field">
        <span>服务名称</span>
        <input v-model="name" type="text" placeholder="可选，例如：本地模型" />
      </label>
      <label class="field">
        <span>API 地址</span>
        <input v-model="serverAddress" type="url" required placeholder="https://api.example.com/v1" />
      </label>
      <label class="field">
        <span>协议格式</span>
        <select v-model="protocol">
          <option value="chat-completions">Chat Completions</option>
        </select>
      </label>
      <p v-if="probedServerAddress" class="muted">聊天接口地址：{{ probedServerAddress }}</p>
      <label class="field">
        <span>API 密钥</span>
        <input v-model="apiKey" type="password" autocomplete="new-password" placeholder="sk-..." />
      </label>
      <p v-if="probing" class="muted">正在自动探测模型…</p>
      <div class="row">
        <button type="button" class="btn-tonal" :disabled="probing" @click="probe">
          {{ probing ? '正在探测…' : '探测模型' }}
        </button>
        <button
          v-if="discoveredModels.length"
          type="button"
          class="btn-secondary"
          :disabled="probing"
          @click="selectionOpen = true"
        >
          修改选择（{{ selectedModels.length }}）
        </button>
      </div>
      <p v-if="probeMessage" class="notice">{{ probeMessage }}</p>
      <div class="row">
        <button type="submit" class="btn" :disabled="saving || probing || selectedModels.length === 0">
          {{ saving ? '正在保存…' : '保存并启用' }}
        </button>
      </div>
    </form>

    <ModelSelectionDialog
      :open="selectionOpen"
      :models="discoveredModels"
      @confirm="acceptModelSelection"
      @cancel="selectionOpen = false"
    />

    <p v-if="store.status" class="muted">{{ store.status }}</p>
    <div v-if="store.error" class="notice">{{ store.error }}</div>

    <div v-if="store.configs.length" class="grid">
      <ConfigCard
        v-for="(item, index) in store.configs"
        :key="index"
        :config="item"
        :index="index"
      />
    </div>
    <div v-else-if="store.loaded && !store.error" class="notice">
      配置文件里还没有任何配置项。
    </div>
  </section>
</template>
