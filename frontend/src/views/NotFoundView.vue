<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <section class="pb-not-found">
    <p class="pb-not-found__code">404</p>
    <h1 class="pb-not-found__title">{{ t('notFound.title') }}</h1>
    <p class="pb-not-found__description">{{ t('notFound.description') }}</p>
    <RouterLink class="pb-not-found__back" to="/">
      {{ t('notFound.backHome') }}
    </RouterLink>
  </section>
</template>

<style scoped>
.pb-not-found {
  padding: var(--pb-space-xl) 0;
  text-align: center;
}

.pb-not-found__code {
  color: var(--pb-color-text-faint);
  font-size: 48px;
  font-weight: 600;
  margin: 0;
}

.pb-not-found__title {
  font-size: 22px;
  margin: var(--pb-space-sm) 0;
}

.pb-not-found__description {
  color: var(--pb-color-text-soft);
  margin: 0 0 var(--pb-space-lg);
}

.pb-not-found__back {
  color: var(--pb-color-primary);
}
</style>
