<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { useChatStore } from './stores/chat'
import { useConfigsStore } from './stores/configs'
import SidebarTasks from './components/SidebarTasks.vue'

const route = useRoute()
const router = useRouter()
const chat = useChatStore()
const store = useConfigsStore()

// 后端就绪 = 配置成功读取过且没有未恢复的错误(状态灯已按需求移除,保留给以后用)。
const ready = computed(() => store.loaded && !store.error)
const pageTitle = computed(() => (route.meta.title as string | undefined) ?? 'CAF')
const mode = computed(() => (route.meta.mode as string | undefined) ?? 'chat')

function newConversation() {
  chat.reset()
  router.push('/chat')
}
</script>

<template>
  <div class="app">
    <aside class="sidebar" aria-label="导航">
      <RouterLink to="/chat" class="brand">
        <h1>CAF</h1>
        <span class="brand-version">v0.1.0</span>
      </RouterLink>

      <!-- 主界面模式：新建对话 + 任务列表 -->
      <template v-if="mode === 'chat'">
        <button
          type="button"
          class="btn new-chat"
          @click="newConversation"
        >
          ＋ 新建对话
        </button>
        <SidebarTasks />
      </template>

      <!-- 设置模式：设置分区导航 -->
      <nav v-else class="nav" aria-label="设置">
        <RouterLink to="/settings/models" class="nav-link">模型配置</RouterLink>
        <RouterLink to="/settings/general" class="nav-link">通用设置</RouterLink>
      </nav>

      <div class="sidebar-foot">
        <RouterLink
          to="/settings/general"
          class="settings-btn"
          :class="{ active: mode === 'settings' }"
          title="设置"
          aria-label="设置"
        >
          <!-- 齿轮图标(Feather, MIT) -->
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <circle cx="12" cy="12" r="3" />
            <path
              d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"
            />
          </svg>
        </RouterLink>
      </div>
    </aside>

    <main class="content stack">
      <header class="page-header">
        <h2>{{ pageTitle }}</h2>
      </header>
      <RouterView />
    </main>
  </div>
</template>
