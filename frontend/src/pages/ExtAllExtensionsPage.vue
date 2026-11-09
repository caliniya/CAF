<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Plus } from '@lucide/vue'
import { useExtensionStore } from '../stores/extensions'
import ExtensionOptionsList from '../components/ExtensionOptionsList.vue'
import ExtensionEditorDialog from '../components/ExtensionEditorDialog.vue'
const ext = useExtensionStore()
const createOpen = ref(false)
let stop: (() => void) | undefined
onMounted(() => { stop = ext.watchOptions() })
onUnmounted(() => stop?.())
</script>
<template><section class="all-extensions"><header><h1>所有扩展</h1><button type="button" class="btn btn-primary" @click="createOpen = true"><Plus :size="14" />添加新的扩展</button></header><div class="page-content"><ExtensionOptionsList /></div><ExtensionEditorDialog :open="createOpen" @close="createOpen = false" /></section></template>
<style scoped>.all-extensions { display:flex;flex-direction:column;flex:1;min-width:0;min-height:0; }header { display:flex;align-items:center;justify-content:space-between;gap:12px;padding:24px 28px 18px;border-bottom:1px solid var(--outline); }h1 { font-size:20px;font-weight:600; }.page-content { flex:1;min-height:0;overflow-y:auto;padding:24px 28px; }@media(max-width:900px) { header,.page-content { padding:20px; } }</style>
