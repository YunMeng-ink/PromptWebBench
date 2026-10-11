<script setup lang="ts">
import en from 'element-plus/es/locale/lang/en'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { computed } from 'vue'
import AppFooter from '@/components/AppFooter.vue'
import AppHeader from '@/components/AppHeader.vue'
import { usePreferencesStore } from '@/stores/preferences'

const preferences = usePreferencesStore()
const elementLocale = computed(() => (preferences.locale === 'zh' ? zhCn : en))
</script>

<template>
  <el-config-provider :locale="elementLocale">
    <div class="pb-shell">
      <AppHeader />
      <main class="pb-main">
        <RouterView />
      </main>
      <AppFooter />
    </div>
  </el-config-provider>
</template>
