import { createRouter, createWebHashHistory } from 'vue-router'
import ChatPage from '../pages/ChatPage.vue'
import ConfigsPage from '../pages/ConfigsPage.vue'
import SettingsGeneralPage from '../pages/SettingsGeneralPage.vue'
import ExtClientsPage from '../pages/ExtClientsPage.vue'
import ExtClientPage from '../pages/ExtClientPage.vue'
import { APP_AREAS, readAppArea } from '../navigation'

// 桌面应用用 hash 路由：不依赖服务器路径重写，刷新后停留在当前页。
// meta.area 标识 Agent 或 Ext；设置页保留进入前选择的界面。
const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: () => APP_AREAS[readAppArea()].path },
    { path: '/chat', component: ChatPage, meta: { title: '对话', mode: 'chat', area: 'agent' } },
    { path: '/ext', component: ExtClientsPage, meta: { title: '客户端', area: 'ext' } },
    { path: '/ext/extensions', redirect: '/ext' },
    { path: '/ext/clients/:clientId', component: ExtClientPage, meta: { title: '编辑客户端', area: 'ext' } },
    { path: '/ext/clients/:clientId/qq/:id', redirect: '/ext' },
    { path: '/qq', redirect: '/ext' },
    { path: '/qq/clients', redirect: '/ext' },
    { path: '/qq/clients/:id', redirect: '/ext' },
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
