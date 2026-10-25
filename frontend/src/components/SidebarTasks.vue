<script setup lang="ts">
import { ref } from 'vue'

// 演示用的假数据:后面接入真实对话列表时整体替换。
// 组织方式:工作区 → 该工作区下已开始的对话,按最近排列。
interface ConvItem {
  id: string
  title: string
}
interface Workspace {
  name: string
  conversations: ConvItem[]
}

const workspaces: Workspace[] = [
  {
    name: '默认工作区',
    conversations: [
      { id: 'c1', title: '调试 Chat Completions 编码' },
      { id: 'c2', title: '任务列表设计' },
    ],
  },
  {
    name: 'CAF 开发',
    conversations: [
      { id: 'c3', title: 'DecodeEvent 状态机' },
      { id: 'c4', title: 'SSE 测试样本分析' },
      { id: 'c5', title: 'QQ 登录流程' },
    ],
  },
]

const activeId = ref('c1')

// 每个工作区的折叠状态;默认全部展开。
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
  <div class="task-list">
    <p class="task-label">任务列表</p>
    <div v-for="ws in workspaces" :key="ws.name" class="task-workspace">
      <button
        type="button"
        class="task-ws-name"
        :aria-expanded="!collapsed.has(ws.name)"
        @click="toggle(ws.name)"
      >
        <span class="task-chevron" :class="{ open: !collapsed.has(ws.name) }">▸</span>
        {{ ws.name }}
        <span class="task-count">{{ ws.conversations.length }}</span>
      </button>
      <template v-if="!collapsed.has(ws.name)">
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
