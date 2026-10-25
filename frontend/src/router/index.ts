import { createRouter, createWebHashHistory } from 'vue-router'
import ChatPage from '../pages/ChatPage.vue'
import ConfigsPage from '../pages/ConfigsPage.vue'
import SettingsGeneralPage from '../pages/SettingsGeneralPage.vue'

// 桌面应用用 hash 路由：不依赖服务器路径重写，刷新后停留在当前页。
// meta.mode 决定侧边栏形态：chat=主界面（新建对话），settings=设置（双项导航）。
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/chat' },
    { path: '/chat', component: ChatPage, meta: { title: '对话', mode: 'chat' } },
    {
      path: '/settings/models',
      component: ConfigsPage,
      meta: { title: '模型配置', mode: 'settings' },
    },
    {
      path: '/settings/general',
      component: SettingsGeneralPage,
      meta: { title: '通用设置', mode: 'settings' },
    },
  ],
})

export default router
