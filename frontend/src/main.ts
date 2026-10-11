import { VueQueryPlugin } from '@tanstack/vue-query'
import ElementPlus from 'element-plus'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import '@/styles/tokens.css'
import App from '@/App.vue'
import { i18n } from '@/i18n'
import { router } from '@/router'
import { useAuthStore } from '@/stores/auth'
import { usePreferencesStore } from '@/stores/preferences'

const app = createApp(App)

app.use(createPinia())
app.use(i18n)
app.use(router)
app.use(ElementPlus)
app.use(VueQueryPlugin)

// 界面偏好需要在首屏渲染前落地，401 处理需要尽早接入。
usePreferencesStore()
useAuthStore().connect()

app.mount('#app')
