<script setup lang="ts">
import { onUnmounted, watch } from 'vue'
import type { ExtensionClient } from '../api'
import { useExtensionStore } from '../stores/extensions'
import ExtensionWindow from './ExtensionWindow.vue'
import ExtensionOptionsList from './ExtensionOptionsList.vue'
const props = defineProps<{ open: boolean; client?: ExtensionClient }>()
const emit = defineEmits<{ close: [] }>()
const ext = useExtensionStore()
let stop: (() => void) | undefined
watch(() => props.open, (open) => { stop?.(); stop = open ? ext.watchOptions() : undefined }, { immediate:true })
onUnmounted(() => stop?.())
</script>
<template><ExtensionWindow :open="open" :title="(client?.name ?? '') + ' · 扩展'" wide @close="emit('close')"><ExtensionOptionsList v-if="client" :client-id="client.id" /></ExtensionWindow></template>
