<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const year = new Date().getFullYear()
</script>

<template>
  <footer class="pb-footer">
    <div class="pb-container pb-footer__inner">
      <span>{{ t('footer.copyright') }} © {{ year }}</span>
    </div>
  </footer>
</template>

<style scoped>
.pb-footer {
  border-top: 1px solid var(--pb-color-border);
  background-color: var(--pb-color-bg-soft);
  color: var(--pb-color-text-faint);
  font-size: 13px;
}

.pb-footer__inner {
  display: flex;
  align-items: center;
  height: 52px;
}
</style>
