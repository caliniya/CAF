<script setup lang="ts">
import { ref } from 'vue'
import { useChatStore } from '../stores/chat'

// 侧边栏任务列表:按 工作区 → 对话 两级组织。
// 数据来自 chat store(演示阶段由「新建工作区」按钮产生),为空时整块隐藏。
const chat = useChatStore()

const activeId = ref<number | null>(null)

// 每个工作区的折叠状态;有对话时才显示折叠箭头。
const collapsed = ref(new Set<string>())

function toggle(name: string) {
  const next = new Set(collapsed.value)
  if (next.has(name)) {
    next.delete(name)
  } else {
    next.add(name)
  }
  collapsed.value = next
}
</script>

<template>
  <div v-if="chat.workspaces.length" class="task-list">
    <p class="task-label">任务列表</p>
    <div v-for="ws in chat.workspaces" :key="ws.name" class="task-workspace">
      <button
        type="button"
        class="task-ws-name"
        :aria-expanded="ws.conversations.length > 0 && !collapsed.has(ws.name)"
        @click="toggle(ws.name)"
      >
        <!-- 文件夹图标(Feather, MIT) -->
        <svg
          class="task-folder"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
        </svg>
        {{ ws.name }}
        <template v-if="ws.conversations.length > 0">
          <span class="task-chevron" :class="{ open: !collapsed.has(ws.name) }">▸</span>
          <span class="task-count">{{ ws.conversations.length }}</span>
        </template>
      </button>
      <template v-if="ws.conversations.length > 0 && !collapsed.has(ws.name)">
        <button
          v-for="c in ws.conversations"
          :key="c.id"
          type="button"
          class="task-item"
          :class="{ active: c.id === activeId }"
          @click="activeId = c.id"
        >
          {{ c.title }}
        </button>
      </template>
    </div>
  </div>
</template>
