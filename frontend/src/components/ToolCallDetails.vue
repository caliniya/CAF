<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { SubmitUserAnswer } from '../api'
import type { ProcessItem } from '../stores/chat'

const props = defineProps<{ item: ProcessItem; chatId?: string }>()
const draft = ref('')
const submitting = ref(false)
const submitted = ref(false)
const error = ref('')
const expanded = ref(false)

watch(() => props.item, () => {
  draft.value = ''
  submitted.value = false
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
const hasDetails = computed(() => Boolean(props.item.displayType || props.item.error))
const infoContent = computed(() => {
  const source = props.item.displayContent ?? ''
  if (!source.startsWith('arguments.')) return props.item.content
  try {
    const value = JSON.parse(props.item.content ?? '{}')?.[source.slice('arguments.'.length)]
    return typeof value === 'string' ? value : undefined
  } catch { return undefined }
})

watch(() => [props.item.status, props.item.displayType, props.item.displayContent], () => {
  expanded.value = props.item.status === 'run'
    && (props.item.displayType === 'question' || (props.item.displayType === 'info' && props.item.displayContent === 'arguments'))
}, { immediate: true })

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

function onToggle(event: Event) {
  expanded.value = (event.target as HTMLDetailsElement).open
}
</script>

<template>
  <details v-if="hasDetails" class="tool-details" :open="expanded" @toggle="onToggle">
    <summary class="tool-line">
      <span class="tool-dot" :class="item.status" aria-hidden="true"></span>
      <span>{{ waiting ? '等待回答' : '执行' }} {{ item.name }}</span>
    </summary>
    <div class="tool-content">
      <template v-if="item.displayType === 'question'">
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
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: .3; } }
</style>
