<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useConfigsStore } from '../stores/configs'
import ModelSelectionDialog from '../components/ModelSelectionDialog.vue'
import type { Model } from '../api'

const store = useConfigsStore()

// 右侧详情展示哪一个供应商
const activeIndex = ref(0)
const activeConfig = computed(() => store.configs[activeIndex.value] ?? null)

// 新建与编辑共用同一个弹窗
const dialogOpen = ref(false)
const editingIndex = ref(-1)
const saving = ref(false)
const probing = ref(false)
const probeMessage = ref('')
const probedServerAddress = ref('')

const form = ref({ name: '', serverAddress: '', apiKey: '' })
const discovered = ref<Model[]>([])
const selected = ref<string[]>([])
const pickerOpen = ref(false)
const confirmDelete = ref(false)

const editing = computed(() => editingIndex.value >= 0)
// 探测要求地址与密钥都填了（编辑时用的是已保存的密钥）
const canProbe = computed(() => !!form.value.serverAddress.trim() && !!form.value.apiKey.trim())
const canSave = computed(
  () => !!form.value.name.trim() && !!form.value.serverAddress.trim() && selected.value.length > 0,
)

onMounted(() => {
  if (!store.loaded) store.load()
})

function resetForm() {
  form.value = { name: '', serverAddress: '', apiKey: '' }
  discovered.value = []
  selected.value = []
  probedServerAddress.value = ''
  probeMessage.value = ''
}

function openCreate() {
  editingIndex.value = -1
  resetForm()
  dialogOpen.value = true
}

function openEdit(index: number) {
  const config = store.configs[index]
  if (!config) return
  editingIndex.value = index
  form.value = {
    name: config.Name ?? '',
    serverAddress: config.ServerAddress ?? '',
    apiKey: config.APIKey ?? '',
  }
  discovered.value = config.Model ?? []
  selected.value = (config.Model ?? []).map((model) => model.id)
  probedServerAddress.value = ''
  probeMessage.value = ''
  dialogOpen.value = true
}

function closeDialog() {
  dialogOpen.value = false
  pickerOpen.value = false
  editingIndex.value = -1
  resetForm()
}

async function probe() {
  if (!canProbe.value || probing.value) return
  probing.value = true
  probeMessage.value = ''
  try {
    const result = await store.probeModels(form.value.serverAddress.trim(), form.value.apiKey)
    const models = Array.isArray(result?.models) ? result.models : []
    probedServerAddress.value = result?.serverAddress || form.value.serverAddress.trim()
    discovered.value = models
    pickerOpen.value = true
    if (!models.length) probeMessage.value = '接口没有返回模型，可在弹窗里手动添加。'
  } catch (err) {
    discovered.value = []
    pickerOpen.value = true
    probeMessage.value = `探测失败：${String(err)}`
  } finally {
    probing.value = false
  }
}

function acceptSelection(picked: Model[], available: Model[]) {
  discovered.value = available
  selected.value = picked.map((model) => model.id)
  pickerOpen.value = false
  probeMessage.value = `已选择 ${picked.length} 个模型`
}

async function submit() {
  if (!canSave.value || saving.value) return
  saving.value = true
  probeMessage.value = ''
  try {
    const models = discovered.value.filter((model) => selected.value.includes(model.id))
    if (editing.value) {
      const index = editingIndex.value
      const name = form.value.name.trim()
      if (name !== store.configs[index]?.Name) await store.rename(index, name)
      await store.refreshModelConfig(index, form.value.serverAddress.trim(), models)
      activeIndex.value = index
    } else {
      await store.addModelConfig(
        form.value.name.trim(),
        probedServerAddress.value || form.value.serverAddress.trim(),
        'chat-completions',
        form.value.apiKey,
        models,
      )
      activeIndex.value = Math.max(0, store.configs.length - 1)
    }
    if (!store.error) closeDialog()
  } catch (err) {
    probeMessage.value = String(err)
  } finally {
    saving.value = false
  }
}

// 把某个供应商设为全局默认；当前已打开的对话不受影响
async function enableConfig() {
  const index = activeIndex.value
  if (!store.configs[index]) return
  await store.selectConfig(index)
}

function askDelete() {
  confirmDelete.value = true
}

async function doDelete() {
  confirmDelete.value = false
  const index = editingIndex.value
  if (index < 0) return
  await store.deleteConfig(index)
  closeDialog()
  activeIndex.value = Math.min(activeIndex.value, Math.max(0, store.configs.length - 1))
}

// 右侧点某个模型 = 把它设为当前使用的模型
function useModel(modelId: string) {
  const config = activeConfig.value
  if (!config) return
  store.selectModel(store.configs.indexOf(config), modelId)
}

function isInUse(index: number) {
  return !!store.enables && store.enables.ID === store.configs[index]?.ID
}

function initial(text: string | undefined) {
  return (text?.trim()?.[0] ?? '?').toUpperCase()
}

function contextLabel(model: Model) {
  if (!model.context_window) return ''
  return `${Math.round(model.context_window / 1000)}K`
}
</script>

<template>
  <section class="page">
    <header class="head">
      <div class="head-text">
        <h1>模型设置</h1>
        <p class="sub">管理模型供应商，配置后可在对话中选择使用。</p>
      </div>
      <button type="button" class="btn btn-primary" @click="openCreate">
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <line x1="12" y1="5" x2="12" y2="19" />
          <line x1="5" y1="12" x2="19" y2="12" />
        </svg>
        添加模型
      </button>
    </header>

    <div class="board">
      <!-- 左：供应商列表 -->
      <aside class="list">
        <div
          v-for="(config, index) in store.configs"
          :key="config.ID"
          class="row"
          :class="{ active: index === activeIndex }"
        >
          <button type="button" class="row-main" @click="activeIndex = index">
            <span class="avatar">{{ initial(config.Name || config.ServerAddress) }}</span>
            <span class="row-text">
              <span class="row-name" :class="{ accent: isInUse(index) }">
                {{ config.Name || config.ServerAddress || '未命名供应商' }}
              </span>
              <span class="row-sub" :class="{ accent: isInUse(index) }">
                {{ config.EnabledModel || '尚未选择模型' }}
              </span>
            </span>
            <span class="dot" :class="{ on: !!config.EnabledModel }"></span>
          </button>
        </div>

        <p v-if="store.loaded && !store.configs.length" class="empty">还没有配置供应商</p>
      </aside>

      <!-- 右：选中供应商详情 -->
      <div class="detail">
        <template v-if="activeConfig">
          <header class="detail-head">
            <span class="avatar lg">{{ initial(activeConfig.Name || activeConfig.ServerAddress) }}</span>
            <div class="detail-title">
              <div class="title-line">
                <h2 :class="{ accent: isInUse(activeIndex) }">
                  {{ activeConfig.Name || '未命名供应商' }}
                </h2>
                <span v-if="isInUse(activeIndex)" class="tag">当前使用</span>
              </div>
              <p class="mono sub">{{ activeConfig.ServerAddress }}</p>
            </div>
            <div class="actions">
              <button
                type="button"
                class="btn btn-sm"
                :disabled="isInUse(activeIndex)"
                :title="isInUse(activeIndex) ? '已经是全局默认配置' : '设为全局默认配置'"
                @click="enableConfig"
              >
                {{ isInUse(activeIndex) ? '已启用' : '启用此配置' }}
              </button>
              <button type="button" class="btn btn-sm" @click="openEdit(activeIndex)">编辑</button>
            </div>
          </header>

          <p class="section-label">模型</p>
          <div class="models">
            <button
              v-for="model in activeConfig.Model ?? []"
              :key="model.id"
              type="button"
              class="model"
              :class="{ on: model.id === activeConfig.EnabledModel, dim: !isInUse(activeIndex) }"
              @click="useModel(model.id)"
            >
              <span class="model-name">{{ model.name || model.id }}</span>
              <span v-if="contextLabel(model)" class="model-meta">{{ contextLabel(model) }}</span>
              <svg
                v-if="model.id === activeConfig.EnabledModel && isInUse(activeIndex)"
                class="check"
                viewBox="0 0 24 24"
                aria-hidden="true"
              >
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </button>
            <p v-if="!activeConfig.Model?.length" class="empty">
              还没有模型，点这一项的「⋯」重新探测。
            </p>
          </div>
        </template>

        <p v-else class="empty big">从左侧选择一个供应商</p>
      </div>
    </div>

    <p v-if="store.status" class="status">{{ store.status }}</p>
    <p v-if="store.error" class="status error">{{ store.error }}</p>
  </section>

  <!-- 新建 / 编辑：同一个弹窗 -->
  <Teleport to="body">
    <div v-if="dialogOpen" class="overlay" @click.self="closeDialog">
      <div class="dialog" role="dialog" aria-modal="true">
        <header class="dialog-head">
          <h2>{{ editing ? '编辑模型供应商' : '添加模型供应商' }}</h2>
          <p class="sub">兼容 OpenAI Chat Completions 的第三方服务或内网网关。</p>
        </header>

        <div class="body">
          <label class="field">
            <span>显示名称</span>
            <input v-model="form.name" type="text" placeholder="例如：本地 Ollama" />
          </label>

          <label class="field">
            <span>Base URL</span>
            <input v-model="form.serverAddress" type="url" placeholder="https://api.example.com/v1" />
          </label>

          <label v-if="!editing" class="field">
            <span>API Key</span>
            <input v-model="form.apiKey" type="password" autocomplete="new-password" placeholder="sk-..." />
          </label>

          <div class="field">
            <span>模型列表</span>
            <div class="models-box">
              <div class="chips">
                <span v-for="id in selected" :key="id" class="chip">{{ id }}</span>
                <span v-if="!selected.length" class="muted">尚未添加模型</span>
              </div>
              <button
                type="button"
                class="btn btn-sm"
                :disabled="!canProbe || probing"
                :title="canProbe ? '' : '先填写 Base URL 和 API Key'"
                @click="probe"
              >
                {{ probing ? '探测中…' : '自动探测' }}
              </button>
            </div>
          </div>

          <p v-if="probeMessage" class="hint">{{ probeMessage }}</p>
        </div>

        <footer class="dialog-foot">
          <button
            v-if="editing"
            type="button"
            class="btn btn-danger-outline btn-sm"
            @click="askDelete"
          >
            删除此配置
          </button>
          <span class="foot-spacer"></span>
          <button type="button" class="btn btn-ghost" @click="closeDialog">取消</button>
          <button type="button" class="btn btn-primary" :disabled="!canSave || saving" @click="submit">
            {{ saving ? '保存中…' : editing ? '保存' : '添加' }}
          </button>
        </footer>
      </div>
    </div>

    <!-- 删除确认 -->
    <div v-if="confirmDelete" class="overlay overlay-top" @click.self="confirmDelete = false">
      <div class="confirm" role="alertdialog" aria-modal="true">
        <h2>删除这个配置？</h2>
        <p class="sub">
          「{{ store.configs[editingIndex]?.Name || '未命名供应商' }}」和它保存的模型列表会一起移除，
          此操作不可撤销。
        </p>
        <div class="confirm-foot">
          <button type="button" class="btn btn-ghost" @click="confirmDelete = false">取消</button>
          <button type="button" class="btn btn-danger" @click="doDelete">删除</button>
        </div>
      </div>
    </div>
  </Teleport>

  <ModelSelectionDialog
    :open="pickerOpen"
    :models="discovered"
    @confirm="acceptSelection"
    @cancel="pickerOpen = false"
  />
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding: 26px 30px 18px;
}

.head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding-bottom: 16px;
}

.head-text h1 {
  font-size: 21px;
  font-weight: 600;
}

.sub {
  margin-top: 4px;
  color: var(--muted);
  font-size: 12.5px;
}

.mono {
  font-family: var(--font-mono);
}

/* ===== 主体两栏 ===== */
.board {
  display: flex;
  flex: 1;
  min-height: 0;
  border: 1px solid var(--outline);
  border-radius: 14px;
  background: var(--surface);
  overflow: hidden;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex-shrink: 0;
  width: 236px;
  padding: 8px;
  border-right: 1px solid var(--outline);
  overflow-y: auto;
}

.row {
  position: relative;
  display: flex;
  align-items: center;
  flex-shrink: 0;
  border-radius: 9px;
  transition: background-color 120ms ease;
}

.row:hover,
.row.active {
  background: var(--tonal);
}

.row-main {
  display: flex;
  align-items: center;
  gap: 9px;
  flex: 1;
  min-width: 0;
  padding: 7px 10px 7px 8px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  text-align: left;
}

.avatar {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  border-radius: 8px;
  background: var(--page);
  color: var(--muted);
  font-size: 12px;
  font-weight: 600;
}

.row.active .avatar {
  background: var(--surface);
  color: var(--accent);
}

.avatar.lg {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  font-size: 14px;
}

.row-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.row-name {
  overflow: hidden;
  color: var(--ink);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 当前启用的供应商：名字与模型都用主色标出来 */
.row-name.accent {
  color: var(--accent);
  font-weight: 500;
}

.row-sub {
  overflow: hidden;
  color: var(--muted);
  font-size: 11.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-sub.accent {
  color: var(--accent);
  opacity: 0.75;
}

.dot {
  width: 6px;
  height: 6px;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--outline);
}

.dot.on {
  background: var(--success);
}

/* ===== 右侧详情 ===== */
.detail {
  flex: 1;
  min-width: 0;
  padding: 20px 22px;
  overflow-y: auto;
}

.detail-head {
  display: flex;
  align-items: center;
  gap: 11px;
  padding-bottom: 18px;
  border-bottom: 1px solid var(--outline);
}

.detail-title {
  flex: 1;
  min-width: 0;
}

.actions {
  display: flex;
  flex-shrink: 0;
  gap: 6px;
}

.title-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  /* 固定行高：徽章出现/消失都不改变这一行的高度，避免画面抖动 */
  min-height: 24px;
}

.detail-title h2 {
  overflow: hidden;
  font-size: 15px;
  font-weight: 600;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-title h2.accent {
  color: var(--accent);
}

.tag {
  flex-shrink: 0;
  padding: 2px 9px;
  border-radius: 999px;
  background: var(--tonal);
  color: var(--accent);
  font-size: 11.5px;
  font-weight: 500;
  /* 高度压在标题行高以内，撑不开这一行 */
  line-height: 1.6;
}

.section-label {
  margin: 18px 0 8px;
  color: var(--muted);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.models {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.model {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 36px;
  padding: 6px 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--ink);
  font-size: 13px;
  text-align: left;
  transition: background-color 120ms ease;
}

.model:hover {
  background: var(--tonal);
}

.model.dim {
  color: var(--muted);
}

.model.on {
  background: var(--tonal);
  color: var(--accent);
}

.model-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-meta {
  flex-shrink: 0;
  color: var(--muted);
  font-size: 11.5px;
}

.check {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
  fill: none;
  stroke: currentColor;
  stroke-width: 2.2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.empty {
  padding: 8px 10px;
  color: var(--muted);
  font-size: 12.5px;
}

.empty.big {
  display: grid;
  place-items: center;
  height: 100%;
  font-size: 13px;
}

.status {
  padding-top: 10px;
  color: var(--muted);
  font-size: 12px;
}

.status.error {
  color: var(--danger);
}

/* ===== 弹窗 ===== */
.overlay {
  position: fixed;
  inset: 0;
  z-index: 200;
  display: grid;
  place-items: center;
  padding: 20px;
  background: rgb(22 29 50 / 42%);
}

.dialog {
  display: flex;
  flex-direction: column;
  width: min(520px, 100%);
  max-height: 86vh;
  border-radius: 14px;
  background: var(--surface);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
  overflow: hidden;
}

.dialog-head {
  padding: 20px 22px 12px;
}

.dialog-head h2 {
  font-size: 16px;
  font-weight: 600;
}

.body {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 4px 22px 8px;
  overflow-y: auto;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field > span {
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 500;
}

.field input {
  height: 36px;
  padding: 0 10px;
  border: 1px solid var(--outline);
  border-radius: 8px;
  background: var(--surface);
  color: var(--ink);
  font-size: 13px;
}

.field input:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--tonal);
}

.models-box {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  padding: 5px 6px 5px 10px;
  border: 1px dashed var(--outline);
  border-radius: 8px;
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  flex: 1;
  min-width: 0;
}

.chip {
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--tonal);
  color: var(--accent);
  font-size: 11.5px;
}

.muted {
  color: var(--muted);
  font-size: 12px;
}

.hint {
  color: var(--muted);
  font-size: 12px;
}

.dialog-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 22px 18px;
}

.foot-spacer {
  flex: 1;
}

/* 删除确认：叠在编辑弹窗之上 */
.overlay-top {
  z-index: 300;
}

.confirm {
  width: min(380px, 100%);
  padding: 20px 22px 16px;
  border-radius: 14px;
  background: var(--surface);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
}

.confirm h2 {
  font-size: 15px;
  font-weight: 600;
}

.confirm .sub {
  margin-top: 8px;
  line-height: 1.6;
}

.confirm-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 20px;
}
</style>
