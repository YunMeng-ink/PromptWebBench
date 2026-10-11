<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { type AppLocale, SUPPORTED_LOCALES } from '@/i18n'
import { usePreferencesStore } from '@/stores/preferences'

const { t } = useI18n()
const preferences = usePreferencesStore()

const localeOptions = computed(() =>
  SUPPORTED_LOCALES.map((value) => ({
    value,
    label: value === 'zh' ? '中文' : 'English',
  })),
)

function onLocaleChange(value: AppLocale): void {
  preferences.setLocale(value)
}
</script>

<template>
  <header class="pb-header">
    <div class="pb-container pb-header__inner">
      <RouterLink class="pb-brand" to="/">
        <span class="pb-brand__title">{{ t('app.title') }}</span>
        <span class="pb-brand__tagline">{{ t('app.tagline') }}</span>
      </RouterLink>

      <nav class="pb-header__actions">
        <RouterLink class="pb-header__link" to="/">
          {{ t('nav.home') }}
        </RouterLink>
        <el-select
          class="pb-header__locale"
          :model-value="preferences.locale"
          :aria-label="t('nav.language')"
          size="small"
          @change="onLocaleChange"
        >
          <el-option
            v-for="option in localeOptions"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          />
        </el-select>
        <el-button size="small" :aria-label="t('nav.theme')" @click="preferences.toggleTheme()">
          {{ preferences.isDark ? t('nav.themeLight') : t('nav.themeDark') }}
        </el-button>
      </nav>
    </div>
  </header>
</template>

<style scoped>
.pb-header {
  position: sticky;
  top: 0;
  z-index: 10;
  height: var(--pb-header-height);
  background-color: var(--pb-color-surface);
  border-bottom: 1px solid var(--pb-color-border);
}

.pb-header__inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  gap: var(--pb-space-md);
}

.pb-brand {
  display: flex;
  align-items: baseline;
  gap: var(--pb-space-sm);
  min-width: 0;
}

.pb-brand__title {
  font-size: 18px;
  font-weight: 600;
  white-space: nowrap;
}

.pb-brand__tagline {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pb-header__actions {
  display: flex;
  align-items: center;
  gap: var(--pb-space-sm);
}

.pb-header__link {
  color: var(--pb-color-text-soft);
  font-size: 14px;
  padding: 0 var(--pb-space-sm);
}

.pb-header__link:hover {
  color: var(--pb-color-primary);
}

.pb-header__locale {
  width: 108px;
}

@media (max-width: 640px) {
  .pb-brand__tagline {
    display: none;
  }
}
</style>
