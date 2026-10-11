<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <section class="pb-home">
    <h1 class="pb-home__title">{{ t('app.title') }}</h1>
    <p class="pb-home__placeholder">{{ t('home.placeholder') }}</p>
  </section>
</template>

<style scoped>
.pb-home {
  padding: var(--pb-space-xl) 0;
}

.pb-home__title {
  font-size: 28px;
  margin: 0 0 var(--pb-space-sm);
}

.pb-home__placeholder {
  color: var(--pb-color-text-soft);
  margin: 0;
}
</style>
