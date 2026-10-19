<script setup lang="ts">
import en from 'element-plus/es/locale/lang/en'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { computed } from 'vue'
import AppFooter from '@/components/AppFooter.vue'
import AppHeader from '@/components/AppHeader.vue'
import { usePreferencesStore } from '@/stores/preferences'

const preferences = usePreferencesStore()
const elementLocale = computed(() => (preferences.locale === 'zh' ? zhCn : en))

/**
 * hideLeaving 把离场视图移出无障碍树。
 *
 * 切换期间新旧视图同时在文档中，两份标题与两份正文会同时被读屏软件念出，
 * 按角色查找元素也会同时命中两处。离场视图只承担视觉上的淡出，不再参与
 * 语义与键盘操作。
 */
function hideLeaving(el: Element): void {
  el.setAttribute('aria-hidden', 'true')
  if (el instanceof HTMLElement) {
    el.inert = true
  }
}
</script>

<template>
  <el-config-provider :locale="elementLocale">
    <div class="pb-shell">
      <AppHeader />
      <main class="pb-main">
        <div class="pb-main__slot">
          <RouterView v-slot="{ Component }">
            <Transition name="pb-page" @leave="hideLeaving">
              <component :is="Component" />
            </Transition>
          </RouterView>
        </div>
      </main>
      <AppFooter />
    </div>
  </el-config-provider>
</template>
