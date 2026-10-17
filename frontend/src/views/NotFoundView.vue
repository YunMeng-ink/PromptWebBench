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
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--pb-space-3xl) 0;
  text-align: center;
}

.pb-not-found__code {
  font-family: var(--pb-font-display);
  font-size: 96px;
  font-weight: 400;
  line-height: 1;
  letter-spacing: -0.04em;
  margin: 0;
  color: var(--pb-accent);
}

.pb-not-found__title {
  margin: var(--pb-space-md) 0 var(--pb-space-sm);
  font-size: 24px;
  font-weight: 600;
}

.pb-not-found__description {
  margin: 0 0 var(--pb-space-xl);
  color: var(--pb-ink-soft);
  font-size: 14.5px;
}

.pb-not-found__back {
  padding-bottom: 2px;
  border-bottom: 1px solid currentColor;
  color: var(--pb-accent);
  font-size: 13.5px;
}

.pb-not-found__back:hover {
  color: var(--pb-accent-strong);
}
</style>
