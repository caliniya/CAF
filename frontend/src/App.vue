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

// 后端就绪 = 配置成功读取过且没有未恢复的错误，用于侧边栏状态指示。
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
      <RouterLink to="/chat" class="brand stack">
        <h1>CAF</h1>
        <p class="muted">Agent 框架</p>
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
          class="settings-link"
          :class="{ active: mode === 'settings' }"
        >
          <span aria-hidden="true">⚙</span> 设置
        </RouterLink>
        <div class="sidebar-meta">
          <span>v0.1.0</span>
          <span class="status-line">
            <span class="dot" :class="{ ready }"></span>
            {{ ready ? '后端就绪' : '连接中' }}
          </span>
        </div>
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
