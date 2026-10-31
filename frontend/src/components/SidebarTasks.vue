<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useChatStore } from '../stores/chat'

// 侧边栏展示后端持久化的会话。
const chat = useChatStore()

const error = ref('')

onMounted(() => {
  chat.loadChats().catch((err) => { error.value = String(err) })
})

</script>

<template>
  <div v-if="chat.chats.length || error" class="task-list">
    <p class="task-label">对话</p>
    <p v-if="error" class="task-error">{{ error }}</p>
    <div v-for="item in chat.chats" :key="item.id" class="task-workspace">
      <button
        type="button"
        class="task-item"
        :class="{ active: item.id === chat.session?.id }"
        @click="chat.openChat(item.id).catch((err) => { error = String(err) })"
      >
        {{ item.title }}
      </button>
      <button
        type="button"
        class="task-delete"
        title="删除对话"
        aria-label="删除对话"
        @click="chat.deleteChat(item.id).catch((err) => { error = String(err) })"
      >
        ×
      </button>
    </div>
  </div>
</template>
