<script setup lang="ts">
// 隐私政策与用户条款（G-10）。静态页面，文案来自语言包，随界面语言切换。
// 两个路由共用本视图，由 mode 决定渲染哪一份文本。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ mode: 'privacy' | 'terms' }>()

const { t, tm } = useI18n()

/** paragraphs 读取键下的段落数组；语言包缺失时返回空数组。 */
function paragraphs(key: string): string[] {
  const value = tm(key)
  if (!Array.isArray(value)) {
    return []
  }
  return value.filter((item): item is string => typeof item === 'string')
}

const sections = computed(() => {
  if (props.mode === 'privacy') {
    return [
      { title: t('privacy.collectTitle'), body: paragraphs('privacy.collectBody') },
      { title: t('privacy.useTitle'), body: paragraphs('privacy.useBody') },
      { title: t('privacy.retentionTitle'), body: paragraphs('privacy.retentionBody') },
      { title: t('privacy.rightsTitle'), body: paragraphs('privacy.rightsBody') },
    ]
  }
  return [
    { title: t('terms.accountTitle'), body: paragraphs('terms.accountBody') },
    { title: t('terms.contentTitle'), body: paragraphs('terms.contentBody') },
    { title: t('terms.conductTitle'), body: paragraphs('terms.conductBody') },
    { title: t('terms.liabilityTitle'), body: paragraphs('terms.liabilityBody') },
  ]
})

const pageTitle = computed(() => (props.mode === 'privacy' ? t('privacy.title') : t('terms.title')))
</script>

<template>
  <article class="pb-legal" data-testid="legal-page">
    <h1 class="pb-legal__title">{{ pageTitle }}</h1>
    <p class="pb-legal__updated">{{ t('legal.updated') }}</p>

    <section v-for="section in sections" :key="section.title" class="pb-legal__section">
      <h2 class="pb-legal__heading">{{ section.title }}</h2>
      <p v-for="line in section.body" :key="line" class="pb-legal__line">{{ line }}</p>
    </section>

    <RouterLink class="pb-legal__back" to="/" data-testid="legal-back">
      {{ t('notFound.backHome') }}
    </RouterLink>
  </article>
</template>

<style scoped>
.pb-legal {
  display: block;
  max-width: 720px;
  padding: var(--pb-space-2xl) 0 var(--pb-space-3xl);
}

.pb-legal__title {
  font-family: var(--pb-font-display);
  font-size: 36px;
  font-weight: 400;
  letter-spacing: -0.01em;
  margin: 0 0 var(--pb-space-xs);
}

.pb-legal__updated {
  margin: 0 0 var(--pb-space-2xl);
  color: var(--pb-ink-faint);
  font-size: 11.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.pb-legal__section {
  margin-bottom: var(--pb-space-xl);
}

.pb-legal__heading {
  margin: 0 0 var(--pb-space-sm);
  font-size: 18px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.pb-legal__line {
  margin: 0 0 10px;
  color: var(--pb-ink-soft);
  font-size: 14.5px;
  line-height: 1.85;
}

.pb-legal__back {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-top: var(--pb-space-lg);
  padding-top: var(--pb-space-lg);
  border-top: 1px solid var(--pb-line);
  color: var(--pb-accent);
  font-size: 13.5px;
  width: 100%;
}

.pb-legal__back:hover {
  color: var(--pb-accent-strong);
}
</style>