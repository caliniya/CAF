import { createRouter, createWebHashHistory } from 'vue-router'
import ChatPage from '../pages/ChatPage.vue'
import ConfigsPage from '../pages/ConfigsPage.vue'

// 桌面应用用 hash 路由：不依赖服务器路径重写，刷新后停留在当前页。
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/chat', component: ChatPage, meta: { title: '对话' } },
    { path: '/configs', component: ConfigsPage, meta: { title: '模型配置' } },
  ],
})

export default router
