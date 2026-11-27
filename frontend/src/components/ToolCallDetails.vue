<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import { ChevronDown } from '@lucide/vue'
import { GetChatImage, SubmitUserAnswer, SubmitToolApproval, type ChatToolConfig, type ToolApprovalChoice } from '../api'
import type { ProcessItem } from '../stores/chat'

const props = defineProps<{ item: ProcessItem; chatId?: string }>()
const emit = defineEmits<{ approved: [value: { chatId: string; config: ChatToolConfig | null }] }>()
const draft = ref('')
const submitting = ref(false)
const submitted = ref(false)
const error = ref('')
const expanded = ref(false)
const approvalSubmitting = ref(false)
const approvalSubmitted = ref(false)
const approvalChoice = ref<ToolApprovalChoice | ''>('')
const approvalMenuOpen = ref(false)
const approvalActions = ref<HTMLElement | null>(null)
const approvalMenu = ref<HTMLElement | null>(null)
const approvalMenuPosition = ref({ left: '0px', top: '0px' })
const approvalMenuId = useId()

watch(() => props.item, () => {
  draft.value = ''
  submitted.value = false
  error.value = ''
})

watch(() => [props.chatId, props.item.callId, props.item.requestId], () => {
  approvalSubmitted.value = false
  approvalSubmitting.value = false
  approvalChoice.value = ''
  approvalMenuOpen.value = false
  error.value = ''
})

const question = computed(() => {
  try {
    const value = JSON.parse(props.item.content ?? '{}')
    return {
      question: typeof value?.question === 'string' ? value.question : '',
      options: Array.isArray(value?.options) ? value.options.filter((option: unknown): option is string => typeof option === 'string') : [],
    }
  } catch { return { question: '', options: [] as string[] } }
})
const waiting = computed(() => props.item.displayType === 'question' && props.item.status === 'run' && !submitted.value)
const approvalWaiting = computed(() => Boolean(props.item.approval && props.item.status === 'run' && props.item.requestId && !approvalSubmitted.value))
const hasDetails = computed(() => Boolean(props.item.displayType || props.item.error || props.item.approval))
const approvalArguments = computed(() => {
  try { return JSON.stringify(JSON.parse(props.item.approval?.arguments ?? '{}'), null, 2) }
  catch { return props.item.approval?.arguments ?? '' }
})
const approvalResult = computed(() => ({
  reject: '已拒绝', once: '已允许本次', workspace: '已允许在此工作区内使用这个工具',
  tool: '已允许此对话使用这个工具', elevate: '已提升对话权限', canceled: '审批已取消',
})[props.item.approval?.decision || approvalChoice.value || 'canceled'])
const infoContent = computed(() => {
  const source = props.item.displayContent ?? ''
  if (!source.startsWith('arguments.')) return props.item.content
  try {
    const value = JSON.parse(props.item.content ?? '{}')?.[source.slice('arguments.'.length)]
    return typeof value === 'string' ? value : undefined
  } catch { return undefined }
})

watch(() => [props.item.status, props.item.displayType, props.item.displayContent, props.item.requestId, props.item.approval?.decision], () => {
  expanded.value = props.item.status === 'run'
    && (approvalWaiting.value || props.item.displayType === 'question' || (props.item.displayType === 'info' && props.item.displayContent === 'arguments'))
  if (!approvalWaiting.value) approvalMenuOpen.value = false
}, { immediate: true })

async function submitApproval(choice: ToolApprovalChoice) {
  if (!approvalWaiting.value || approvalSubmitting.value || !props.chatId || !props.item.requestId) return
  const chatId = props.chatId, requestId = props.item.requestId
  approvalSubmitting.value = true
  approvalMenuOpen.value = false
  error.value = ''
  try {
    const config = await SubmitToolApproval(chatId, requestId, choice)
    emit('approved', { chatId, config })
    if (props.chatId === chatId && props.item.requestId === requestId) {
      approvalChoice.value = choice
      approvalSubmitted.value = true
    }
  } catch (err) {
    if (props.chatId === chatId && props.item.requestId === requestId) error.value = String(err)
  } finally {
    if (props.chatId === chatId && props.item.requestId === requestId) approvalSubmitting.value = false
  }
}

async function toggleApprovalMenu(event: MouseEvent) {
  if (!approvalWaiting.value || approvalSubmitting.value) return
  approvalMenuOpen.value = !approvalMenuOpen.value
  if (!approvalMenuOpen.value) return
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  approvalMenuPosition.value = {
    left: `${Math.max(8, Math.min(bounds.right - 252, window.innerWidth - 260))}px`,
    top: `${bounds.bottom + 5}px`,
  }
  await nextTick()
  const height = approvalMenu.value?.getBoundingClientRect().height ?? 132
  approvalMenuPosition.value.top = `${Math.max(8, bounds.bottom + height + 5 <= window.innerHeight - 8 ? bounds.bottom + 5 : bounds.top - height - 5)}px`
}

function closeApprovalMenu() { approvalMenuOpen.value = false }
function closeApprovalMenuOnOutside(event: PointerEvent) {
  const target = event.target as Node
  if (!approvalActions.value?.contains(target) && !approvalMenu.value?.contains(target)) closeApprovalMenu()
}
function dismissApprovalMenu(event: KeyboardEvent) {
  if (event.key === 'Escape' && approvalMenuOpen.value) {
    event.preventDefault()
    event.stopPropagation()
    closeApprovalMenu()
    approvalActions.value?.querySelector<HTMLButtonElement>('[aria-haspopup="menu"]')?.focus()
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', closeApprovalMenuOnOutside)
  document.addEventListener('scroll', closeApprovalMenu, true)
  window.addEventListener('resize', closeApprovalMenu)
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', closeApprovalMenuOnOutside)
  document.removeEventListener('scroll', closeApprovalMenu, true)
  window.removeEventListener('resize', closeApprovalMenu)
})

async function submitAnswer() {
  if (!draft.value.trim() || !props.chatId || !props.item.requestId || submitting.value || submitted.value) return
  submitting.value = true
  error.value = ''
  try {
    await SubmitUserAnswer(props.chatId, props.item.requestId, draft.value)
    submitted.value = true
    expanded.value = false
  } catch (err) { error.value = String(err) }
  finally { submitting.value = false }
}

const preview = ref(''), previewError = ref(''), loadingImage = ref(false)
let previewVersion = 0
watch(() => [props.chatId, props.item.image?.id], () => {
  ++previewVersion; preview.value = previewError.value = ''; loadingImage.value = false
  if (expanded.value) void loadImage()
})
async function loadImage() {
  const image = props.item.image, chatId = props.chatId
  if (!image || !chatId || preview.value || loadingImage.value) return
  const version = ++previewVersion; loadingImage.value = true; previewError.value = ''
  try {
    const url = await GetChatImage(chatId, image.id)
    if (version === previewVersion && /^data:image\/(png|jpeg);base64,/.test(url)) preview.value = url
  } catch (e) { if (version === previewVersion) previewError.value = String(e) }
  finally { if (version === previewVersion) loadingImage.value = false }
}
function onToggle(event: Event) {
  expanded.value = (event.target as HTMLDetailsElement).open
  if (expanded.value) void loadImage()
  else { ++previewVersion; preview.value = previewError.value = ''; loadingImage.value = false }
}
</script>

<template>
  <details v-if="hasDetails" class="tool-details" :open="expanded" @toggle="onToggle">
    <summary class="tool-line">
      <span class="tool-dot" :class="item.status" aria-hidden="true"></span>
      <span v-if="item.displayType === 'image'">{{ item.imageSent ? '已读取图片' : item.status === 'fail' ? '读取图片失败' : item.image && item.status ? '图片已载入' : item.name }}<template v-if="item.image"> · {{ item.image.name }}</template></span>
      <span v-else>{{ approvalWaiting ? '等待审批' : waiting ? '等待回答' : '执行' }} {{ item.approval?.name || item.name }}</span>
    </summary>
    <div class="tool-content">
      <template v-if="item.displayType === 'image' && item.image">
        <img v-if="preview" class="image-preview" :src="preview" :alt="item.image.name" />
        <span v-else-if="loadingImage" role="status">正在读取预览…</span>
        <p v-if="previewError" class="tool-error" role="alert">{{ previewError }} <button type="button" @click="loadImage">重试</button></p>
      </template>
      <template v-else-if="item.approval">
        <p class="approval-reason">{{ item.approval.reason }}</p>
        <p v-if="item.approval.workspaceName" class="approval-scope">当前工作区：{{ item.approval.workspaceName }}</p>
        <pre class="tool-text approval-arguments">{{ approvalArguments }}</pre>
        <div v-if="approvalWaiting" ref="approvalActions" class="approval-actions" @keydown="dismissApprovalMenu">
          <button type="button" class="btn btn-ghost btn-sm" :disabled="approvalSubmitting" @click="submitApproval('reject')">拒绝</button>
          <div class="approval-allow">
            <button type="button" class="btn btn-primary btn-sm approval-once" :disabled="approvalSubmitting" @click="submitApproval('once')">
              {{ approvalSubmitting ? '提交中…' : '允许本次' }}
            </button>
            <button type="button" class="btn btn-primary btn-sm approval-expand" aria-label="更多允许选项" aria-haspopup="menu"
              :aria-controls="approvalMenuId" :aria-expanded="approvalMenuOpen" :disabled="approvalSubmitting" @click="toggleApprovalMenu">
              <ChevronDown :size="14" :stroke-width="2" />
            </button>
          </div>
        </div>
        <p v-else class="approval-result">{{ approvalResult }}</p>
        <p v-if="error" class="tool-error" role="alert">{{ error }}</p>
        <pre v-if="!approvalWaiting && item.displayContent === 'result' && infoContent !== undefined" class="tool-text approval-output">{{ infoContent }}</pre>
      </template>
      <template v-else-if="item.displayType === 'question'">
        <p class="question-text">{{ question.question || item.content }}</p>
        <form v-if="waiting" @submit.prevent="submitAnswer">
          <div v-if="question.options.length" class="question-options">
            <button v-for="option in question.options" :key="option" type="button" class="question-option"
              :class="{ selected: draft === option }" :aria-pressed="draft === option" :disabled="submitting"
              @click="draft = option">{{ option }}</button>
          </div>
          <textarea v-model="draft" aria-label="你的回答" placeholder="输入你的回答，也可以选择上面的答案" rows="2"
            :disabled="submitting" class="question-input"></textarea>
          <button type="submit" class="btn btn-primary btn-sm" :disabled="!draft.trim() || !item.requestId || submitting">
            {{ submitting ? '提交中…' : '提交回答' }}
          </button>
          <p v-if="error" class="tool-error" role="alert">{{ error }}</p>
        </form>
        <p v-else-if="item.status !== 'fail'" class="tool-text">回答：{{ item.answer ?? draft }}</p>
      </template>
      <pre v-else-if="infoContent !== undefined" class="tool-text">{{ infoContent || '（空内容）' }}</pre>
      <p v-if="item.error" class="tool-error" role="alert">{{ item.error }}</p>
    </div>
  </details>
  <div v-else class="tool-line">
    <span class="tool-dot" :class="item.status" aria-hidden="true"></span>
    执行 {{ item.name }}
  </div>
  <Teleport to="body">
    <div v-if="approvalMenuOpen && approvalWaiting" :id="approvalMenuId" ref="approvalMenu" class="approval-menu" role="menu"
      :style="approvalMenuPosition" @keydown="dismissApprovalMenu">
      <button v-if="item.approval?.workspaceAllowed" type="button" role="menuitem" title="只在当前对话的此工作区内生效" @click="submitApproval('workspace')">允许工作区内使用此工具</button>
      <button type="button" role="menuitem" title="当前对话内使用此工具时无需再次审批" @click="submitApproval('tool')">允许此工具</button>
      <button v-if="item.approval?.higherPermission" type="button" role="menuitem" @click="submitApproval('elevate')">
        切换为更高权限<span class="approval-level">{{ item.approval.higherPermission === 2 ? '自动编辑' : '完全访问' }}</span>
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.tool-line { display: flex; align-items: center; gap: 6px; color: var(--muted); font-family: var(--font-mono); font-size: 12px; }
summary.tool-line { cursor: pointer; list-style: none; }
summary.tool-line::-webkit-details-marker { display: none; }
summary.tool-line::before { content: '▸'; }
.tool-details[open] > summary::before { transform: rotate(90deg); }
.tool-dot { width: 6px; height: 6px; flex-shrink: 0; border-radius: 50%; background: var(--muted); }
.tool-dot.run { background: var(--accent); animation: pulse 1.1s ease-in-out infinite; }
.tool-dot.done { background: #1e7d3c; }
.tool-dot.fail, .tool-error { color: var(--danger); }
.tool-dot.fail { background: var(--danger); }
.tool-content { margin: 8px 0 0 12px; padding: 10px 12px; border: 1px solid var(--outline); border-radius: 8px; }
.tool-text, .question-text { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 13px; line-height: 1.6; }
.tool-text { max-height: 360px; overflow: auto; }
.question-text { margin-bottom: 10px; }
.question-options { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px; }
.question-option { padding: 6px 10px; border: 1px solid var(--outline); border-radius: 7px; background: var(--page); color: var(--ink); text-align: left; white-space: pre-wrap; overflow-wrap: anywhere; }
.question-option.selected { border-color: var(--accent); background: var(--tonal); }
.question-input { display: block; box-sizing: border-box; width: 100%; margin-bottom: 8px; padding: 8px; border: 1px solid var(--outline); border-radius: 7px; background: var(--page); color: var(--ink); font: inherit; resize: vertical; }
.tool-error { margin: 8px 0 0; font-size: 13px; overflow-wrap: anywhere; }
.approval-reason { margin: 0 0 6px; font-size: 13px; line-height: 1.6; }
.approval-scope { margin: 0 0 8px; color: var(--muted); font-size: 12px; }
.approval-arguments { padding: 8px 10px; border-radius: 6px; background: var(--tonal); font-family: var(--font-mono); font-size: 12px; }
.approval-actions { display: flex; justify-content: flex-end; align-items: center; gap: 8px; margin-top: 12px; }
.approval-allow { display: flex; }
.approval-once { border-top-right-radius: 0; border-bottom-right-radius: 0; }
.approval-expand { width: 30px; padding: 0; border-top-left-radius: 0; border-bottom-left-radius: 0; border-left: 1px solid rgb(255 255 255 / 28%); }
.approval-result { margin: 10px 0 0; color: var(--muted); font-size: 12px; }
.approval-output { margin-top: 10px; }
.approval-menu { position: fixed; z-index: 100; width: min(252px, calc(100vw - 16px)); padding: 5px; border: 1px solid var(--outline); border-radius: 9px; background: var(--surface); color: var(--ink); box-shadow: 0 8px 24px rgb(22 29 50 / 18%); }
.approval-menu button { display: flex; justify-content: space-between; align-items: center; gap: 10px; width: 100%; padding: 9px 10px; border: 0; border-radius: 5px; background: transparent; color: inherit; text-align: left; font-size: 12.5px; cursor: pointer; }
.approval-menu button:hover, .approval-menu button:focus-visible { background: var(--tonal); }
.approval-level { color: var(--muted); font-size: 11px; }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .3; } }
.image-preview { display:block; width:auto; height:auto; max-width:100%; max-height:55vh; object-fit:contain; }
</style>
