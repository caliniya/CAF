<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, type ComponentPublicInstance } from 'vue'
import { useChatStore } from '../stores/chat'

// 侧边栏展示后端持久化的会话。
const chat = useChatStore()

const error = ref('')
const openedMenuId = ref('')
const menuPosition = ref<Record<string, string>>({})
const renamingId = ref('')
const renameDraft = ref('')
const renameInput = ref<HTMLInputElement | null>(null)

// v-for 内的字符串 ref 会被收集成数组，改用函数 ref 精确拿到当前正在编辑的输入框。
function setRenameInput(el: Element | ComponentPublicInstance | null) {
  if (el instanceof HTMLInputElement) renameInput.value = el
}

const menuItem = computed(() => chat.chats.find((item) => item.id === openedMenuId.value))

function toggleMenu(id: string, event: MouseEvent) {
  if (openedMenuId.value === id) {
    openedMenuId.value = ''
    return
  }
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const menuWidth = 156
  menuPosition.value = {
    top: `${bounds.top}px`,
    left: `${Math.min(bounds.right + 4, window.innerWidth - menuWidth - 8)}px`,
  }
  openedMenuId.value = id
}

function closeMenuOnOutside(event: PointerEvent) {
  if (!(event.target instanceof Element)) return
  if (event.target.closest('.task-more-wrap, .task-flyout')) return
  openedMenuId.value = ''
}

function closeOnEscape(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  openedMenuId.value = ''
  renamingId.value = ''
}

function beginRename(id: string, title: string) {
  openedMenuId.value = ''
  renameDraft.value = title
  renamingId.value = id
  nextTick(() => {
    renameInput.value?.focus()
    renameInput.value?.select()
  })
}

function cancelRename() {
  renamingId.value = ''
  renameDraft.value = ''
  error.value = ''
}

async function submitRename(id: string, currentTitle: string) {
  const title = renameDraft.value.trim()
  if (!title) {
    error.value = '对话标题不能为空'
    renameInput.value?.focus()
    return
  }
  if (title === currentTitle) {
    cancelRename()
    return
  }
  try {
    await chat.renameChat(id, title)
    cancelRename()
  } catch (err) {
    error.value = String(err)
  }
}

async function deleteChat(id: string) {
  openedMenuId.value = ''
  try {
    await chat.deleteChat(id)
  } catch (err) {
    error.value = String(err)
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', closeMenuOnOutside)
  document.addEventListener('keydown', closeOnEscape)
  chat.loadChats().catch((err) => { error.value = String(err) })
})
onUnmounted(() => {
  document.removeEventListener('pointerdown', closeMenuOnOutside)
  document.removeEventListener('keydown', closeOnEscape)
})
</script>

<template>
  <div v-if="chat.chats.length || error" class="task-list">
    <p class="task-label">对话</p>
    <p v-if="error" class="task-error">{{ error }}</p>

    <div
      v-for="item in chat.chats"
      :key="item.id"
      class="task-row"
      :class="{ active: item.id === chat.session?.id }"
    >
      <div v-if="renamingId === item.id" class="task-rename">
        <input
          :ref="setRenameInput"
          v-model="renameDraft"
          type="text"
          aria-label="对话名称"
          @keydown.enter.prevent="submitRename(item.id, item.title)"
          @keydown.esc.prevent="cancelRename"
        />
        <button type="button" title="保存" aria-label="保存" @click="submitRename(item.id, item.title)">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <polyline points="20 6 9 17 4 12" />
          </svg>
        </button>
        <button type="button" title="取消" aria-label="取消" @click="cancelRename">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <line x1="18" y1="6" x2="6" y2="18" />
            <line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>
      </div>

      <template v-else>
        <button
          type="button"
          class="task-item"
          :class="{ active: item.id === chat.session?.id }"
          :title="item.title"
          @click="chat.openChat(item.id).catch((err) => { error = String(err) })"
        >
          <span class="task-title">{{ item.title }}</span>
        </button>
        <span class="task-more-wrap">
          <button
            type="button"
            class="task-more"
            title="更多操作"
            aria-label="更多操作"
            aria-haspopup="menu"
            :aria-expanded="openedMenuId === item.id"
            @click="toggleMenu(item.id, $event)"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <circle cx="5" cy="12" r="1.6" />
              <circle cx="12" cy="12" r="1.6" />
              <circle cx="19" cy="12" r="1.6" />
            </svg>
          </button>
        </span>
      </template>
    </div>
  </div>

  <Teleport to="body">
    <div
      v-if="menuItem"
      class="task-flyout"
      :style="menuPosition"
      role="menu"
      @keydown.esc.stop="openedMenuId = ''"
    >
      <button type="button" role="menuitem" @click="beginRename(menuItem.id, menuItem.title)">
        重命名
      </button>
      <button type="button" class="danger" role="menuitem" @click="deleteChat(menuItem.id)">
        删除
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.task-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
  min-height: 0;
  margin-top: 16px;
  overflow-y: auto;
}

.task-label {
  padding: 0 10px 6px;
  color: var(--muted);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.task-error {
  padding: 4px 10px;
  color: var(--danger);
  font-size: 12px;
}

.task-row {
  position: relative;
  display: flex;
  align-items: center;
  flex-shrink: 0;
  border-radius: 8px;
  transition: background-color 120ms ease;
}

.task-row:hover,
.task-row.active {
  background: var(--tonal);
}

.task-item {
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 0;
  height: 34px;
  padding: 0 30px 0 10px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--ink);
  font-size: 13.5px;
  text-align: left;
  cursor: pointer;
}

.task-row.active .task-item {
  color: var(--accent);
  font-weight: 500;
}

/* 滚动容器会裁掉外扩的 outline，这里改成向内画 */
.task-item:focus-visible,
.task-more:focus-visible {
  outline-offset: -2px;
}

.task-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-more-wrap {
  position: absolute;
  right: 4px;
  top: 50%;
  transform: translateY(-50%);
}

.task-more {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  opacity: 0;
  transition: opacity 120ms ease, background-color 120ms ease;
}

.task-more svg {
  width: 14px;
  height: 14px;
  fill: currentColor;
}

.task-row:hover .task-more,
.task-more:focus-visible,
.task-more[aria-expanded='true'] {
  opacity: 1;
}

.task-more:hover {
  background: rgb(32 38 55 / 8%);
  color: var(--ink);
}

.task-rename {
  display: flex;
  align-items: center;
  gap: 2px;
  width: 100%;
  padding: 2px;
}

.task-rename input {
  flex: 1;
  min-width: 0;
  height: 26px;
  padding: 0 6px;
  border: 1px solid var(--accent);
  border-radius: 6px;
  background: var(--surface);
  color: var(--ink);
  font-size: 13px;
}

.task-rename button {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
}

.task-rename button:hover {
  background: var(--outline);
  color: var(--ink);
}

.task-rename svg {
  width: 13px;
  height: 13px;
  fill: none;
  stroke: currentColor;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.task-flyout {
  position: fixed;
  z-index: 1000;
  display: grid;
  min-width: 140px;
  padding: 4px;
  border: 1px solid var(--outline);
  border-radius: 8px;
  background: var(--surface);
  box-shadow: 0 8px 24px rgb(32 38 55 / 14%);
}

.task-flyout button {
  width: 100%;
  min-height: 30px;
  padding: 5px 9px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--ink);
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}

.task-flyout button:hover {
  background: var(--tonal);
}

.task-flyout button.danger {
  color: var(--danger);
}
</style>
