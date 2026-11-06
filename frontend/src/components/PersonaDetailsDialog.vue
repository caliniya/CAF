<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { Persona } from '../api'

const props = defineProps<{ persona: Persona | null }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)

watch(
  () => props.persona,
  async (persona) => {
    await nextTick()
    if (props.persona !== persona) return
    if (persona && dialog.value && !dialog.value.open) {
      dialog.value.showModal()
    } else if (!persona && dialog.value?.open) {
      dialog.value.close()
    }
  },
  { immediate: true },
)
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="persona-dialog" aria-labelledby="persona-details-title"
      @click.self="emit('close')" @cancel.prevent="emit('close')" @close="emit('close')">
      <div v-if="persona" class="persona-dialog-body">
        <header class="persona-details-head">
          <div>
            <p class="persona-details-sub">人设详情</p>
            <h2 id="persona-details-title">{{ persona.name }}</h2>
          </div>
          <button type="button" class="btn btn-ghost btn-sm" autofocus @click="emit('close')">关闭</button>
        </header>

        <div class="persona-details-content">
          <dl class="persona-metadata">
            <div>
              <dt>介绍</dt>
              <dd>{{ persona.description || '暂无介绍' }}</dd>
            </div>
            <div>
              <dt>ID</dt>
              <dd class="persona-id">{{ persona.id }}</dd>
            </div>
          </dl>
          <section class="persona-prompt" aria-labelledby="persona-prompt-title">
            <h3 id="persona-prompt-title">提示词</h3>
            <pre>{{ persona.system || '暂无提示词' }}</pre>
          </section>
        </div>
      </div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.persona-dialog {
  width: min(640px, calc(100vw - 40px));
  max-height: calc(100vh - 40px);
  padding: 0;
  border: 1px solid var(--outline);
  border-radius: 14px;
  background: var(--surface);
  color: var(--ink);
  box-shadow: 0 24px 70px rgb(22 29 50 / 26%);
  overflow: hidden;
}

.persona-dialog::backdrop {
  background: rgb(22 29 50 / 42%);
}

.persona-dialog-body {
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 42px);
}

.persona-details-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 20px;
  border-bottom: 1px solid var(--outline);
}

.persona-details-head h2 {
  font-size: 18px;
  font-weight: 600;
}

.persona-details-head button {
  flex-shrink: 0;
}

.persona-details-sub {
  margin-bottom: 3px;
  color: var(--muted);
  font-size: 12px;
}

.persona-details-content {
  min-height: 0;
  padding: 20px;
  overflow-y: auto;
}

.persona-metadata {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin: 0;
}

.persona-metadata dt,
.persona-prompt h3 {
  margin-bottom: 6px;
  color: var(--muted);
  font-size: 12px;
  font-weight: 500;
}

.persona-metadata dd {
  margin: 0;
  font-size: 13px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.persona-id {
  font-family: var(--font-mono);
}

.persona-prompt {
  margin-top: 20px;
}

.persona-prompt pre {
  margin: 0;
  padding: 14px;
  border: 1px solid var(--outline);
  border-radius: 10px;
  background: var(--page);
  font: inherit;
  font-size: 13px;
  line-height: 1.75;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
