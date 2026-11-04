<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useChatStore } from '../stores/chat'

// 侧边栏展示后端持久化的会话。
const chat = useChatStore()

const error = ref('')
const openedMenuId = ref('')
const menuPosition = ref<Record<string, string>>({})
const renamingId = ref('')
const renameDraft = ref('')
const renameInput = ref<HTMLInputElement | null>(null)
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
    <div v-for="item in chat.chats" :key="item.id" class="task-workspace">
      <div v-if="renamingId === item.id" class="task-rename">
        <input
          ref="renameInput"
          v-model="renameDraft"
          type="text"
          aria-label="对话名称"
          @keydown.enter.prevent="submitRename(item.id, item.title)"
          @keydown.esc.prevent="cancelRename"
        />
        <button type="button" class="task-rename-action" title="保存名称" aria-label="保存名称" @click="submitRename(item.id, item.title)">
          ✓
        </button>
        <button type="button" class="task-rename-action" title="取消重命名" aria-label="取消重命名" @click="cancelRename">
          ×
        </button>
      </div>
      <template v-else>
        <button
          type="button"
          class="task-item"
          :class="{ active: item.id === chat.session?.id }"
          @click="chat.openChat(item.id).catch((err) => { error = String(err) })"
        >
          {{ item.title }}
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
            <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
              <circle cx="5" cy="12" r="1.7" />
              <circle cx="12" cy="12" r="1.7" />
              <circle cx="19" cy="12" r="1.7" />
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
