import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './style.css'
import './styles/Button.css'
import './styles/menu.css'

createApp(App).use(createPinia()).use(router).mount('#app')
