<script setup lang="ts">
import { onMounted } from 'vue'
import { useConfigsStore } from '../stores/configs'
import ConfigCard from '../components/ConfigCard.vue'
import CurrentPanel from '../components/CurrentPanel.vue'

const store = useConfigsStore()
onMounted(() => {
  if (!store.loaded) store.load()
})
</script>

<template>
  <section class="card stack page-scroll">
    <p class="description">
      读取数据目录下 config/MainConfig.json 中的模型服务配置；点击「启用」选择提供商，点击模型徽章选定要使用的模型。
      <span v-if="store.configs.length" class="badge">{{ store.configs.length }} 项</span>
    </p>

    <CurrentPanel v-if="store.loaded" />

    <div class="row">
      <button type="button" :disabled="store.loading" @click="store.load()">
        重新加载
      </button>
    </div>

    <p v-if="store.status" class="muted">{{ store.status }}</p>
    <div v-if="store.error" class="notice">{{ store.error }}</div>

    <div v-if="store.configs.length" class="grid">
      <ConfigCard
        v-for="(item, index) in store.configs"
        :key="index"
        :config="item"
        :index="index"
      />
    </div>
    <div v-else-if="store.loaded && !store.error" class="notice">
      配置文件里还没有任何配置项。
    </div>
  </section>
</template>
