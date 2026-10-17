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
      {
        id: 'collect',
        title: t('privacy.collectTitle'),
        body: paragraphs('privacy.collectBody'),
      },
      { id: 'use', title: t('privacy.useTitle'), body: paragraphs('privacy.useBody') },
      {
        id: 'storage',
        title: t('privacy.storageTitle'),
        body: paragraphs('privacy.storageBody'),
      },
      { id: 'share', title: t('privacy.shareTitle'), body: paragraphs('privacy.shareBody') },
      {
        id: 'retention',
        title: t('privacy.retentionTitle'),
        body: paragraphs('privacy.retentionBody'),
      },
      {
        id: 'security',
        title: t('privacy.securityTitle'),
        body: paragraphs('privacy.securityBody'),
      },
      { id: 'rights', title: t('privacy.rightsTitle'), body: paragraphs('privacy.rightsBody') },
      { id: 'minor', title: t('privacy.minorTitle'), body: paragraphs('privacy.minorBody') },
      {
        id: 'update',
        title: t('privacy.updateTitle'),
        body: paragraphs('privacy.updateBody'),
      },
    ]
  }
  return [
    { id: 'account', title: t('terms.accountTitle'), body: paragraphs('terms.accountBody') },
    { id: 'content', title: t('terms.contentTitle'), body: paragraphs('terms.contentBody') },
    { id: 'conduct', title: t('terms.conductTitle'), body: paragraphs('terms.conductBody') },
    {
      id: 'measurement',
      title: t('terms.measurementTitle'),
      body: paragraphs('terms.measurementBody'),
    },
    { id: 'ip', title: t('terms.ipTitle'), body: paragraphs('terms.ipBody') },
    {
      id: 'termination',
      title: t('terms.terminationTitle'),
      body: paragraphs('terms.terminationBody'),
    },
    { id: 'liability', title: t('terms.liabilityTitle'), body: paragraphs('terms.liabilityBody') },
    { id: 'dispute', title: t('terms.disputeTitle'), body: paragraphs('terms.disputeBody') },
    { id: 'contact', title: t('terms.contactTitle'), body: paragraphs('terms.contactBody') },
  ]
})

const pageTitle = computed(() => (props.mode === 'privacy' ? t('privacy.title') : t('terms.title')))
const intro = computed(() => (props.mode === 'privacy' ? t('privacy.intro') : t('terms.intro')))
const eyebrow = computed(() => (props.mode === 'privacy' ? 'Legal / Privacy' : 'Legal / Terms'))
</script>

<template>
  <article class="pb-legal" data-testid="legal-page">
    <header class="pb-legal__hero">
      <span class="pb-eyebrow" aria-hidden="true">{{ eyebrow }}</span>
      <h1 class="pb-legal__title">{{ pageTitle }}</h1>
      <p class="pb-legal__updated">{{ t('legal.updated') }}</p>
      <p class="pb-legal__intro">{{ intro }}</p>
    </header>

    <div class="pb-legal__layout">
      <aside class="pb-legal__toc">
        <span class="pb-legal__toc-label">{{ t('methodology.toc') }}</span>
        <nav class="pb-legal__toc-nav">
          <a v-for="(section, index) in sections" :key="section.id" :href="`#${section.id}`">
            <span class="pb-legal__toc-index">{{ String(index + 1).padStart(2, '0') }}</span>
            {{ section.title }}
          </a>
        </nav>
      </aside>

      <div class="pb-legal__content">
        <section
          v-for="(section, index) in sections"
          :id="section.id"
          :key="section.id"
          class="pb-legal__section"
        >
          <h2 class="pb-legal__heading">
            <span class="pb-legal__index" aria-hidden="true">
              {{ String(index + 1).padStart(2, '0') }}
            </span>
            {{ section.title }}
          </h2>
          <p v-for="line in section.body" :key="line" class="pb-legal__line">{{ line }}</p>
        </section>

        <RouterLink class="pb-legal__back" to="/" data-testid="legal-back">
          {{ t('notFound.backHome') }}
        </RouterLink>
      </div>
    </div>
  </article>
</template>

<style scoped>
.pb-legal {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-2xl);
  padding: var(--pb-space-xl) 0 var(--pb-space-3xl);
}

.pb-legal__hero {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  padding-bottom: var(--pb-space-xl);
  border-bottom: 1px solid var(--pb-line);
}

.pb-legal__hero::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 160px;
  height: 2px;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
}

.pb-legal__title {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: clamp(38px, 5vw, 52px);
  font-weight: 500;
  letter-spacing: -0.02em;
  line-height: 1.1;
}

.pb-legal__updated {
  margin: 0;
  color: var(--pb-ink-faint);
  font-size: 10.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.pb-legal__intro {
  max-width: 72ch;
  margin: var(--pb-space-sm) 0 0;
  color: var(--pb-ink-soft);
  font-size: 15px;
  line-height: 1.85;
}

.pb-legal__layout {
  display: grid;
  grid-template-columns: 200px minmax(0, 760px);
  gap: var(--pb-space-2xl);
  align-items: start;
}

.pb-legal__toc {
  position: sticky;
  top: calc(var(--pb-header-height) + var(--pb-space-lg));
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  min-width: 0;
  padding-top: var(--pb-space-xs);
}

.pb-legal__toc-label {
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.pb-legal__toc-nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  min-width: 0;
}

.pb-legal__toc-nav a {
  display: flex;
  align-items: baseline;
  gap: 9px;
  padding: 5px 8px 5px 0;
  border-left: 1px solid transparent;
  color: var(--pb-ink-soft);
  font-size: 12.5px;
  line-height: 1.5;
  transition:
    color var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease),
    padding-left var(--pb-duration) var(--pb-ease);
}

.pb-legal__toc-nav a:hover {
  padding-left: 8px;
  border-left-color: var(--pb-accent);
  color: var(--pb-accent);
}

.pb-legal__toc-index {
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-mono);
  font-size: 9.5px;
  letter-spacing: 0.04em;
}

.pb-legal__content {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xl);
  min-width: 0;
}

.pb-legal__section {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  min-width: 0;
  scroll-margin-top: calc(var(--pb-header-height) + var(--pb-space-lg));
}

.pb-legal__heading {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin: 0 0 var(--pb-space-xs);
  font-family: var(--pb-font-display);
  font-size: 26px;
  font-weight: 500;
  letter-spacing: -0.012em;
  line-height: 1.25;
}

.pb-legal__index {
  color: var(--pb-accent);
  font-family: var(--pb-font-mono);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.06em;
}

.pb-legal__line {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 14.5px;
  line-height: 1.9;
}

.pb-legal__back {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  margin-top: var(--pb-space-sm);
  padding-top: var(--pb-space-lg);
  border-top: 1px solid var(--pb-line);
  color: var(--pb-accent);
  font-size: 13.5px;
}

.pb-legal__back::before {
  content: "";
  width: 16px;
  height: 1px;
  background: currentColor;
  opacity: 0.6;
}

.pb-legal__back:hover {
  color: var(--pb-accent-strong);
}

@media (max-width: 900px) {
  .pb-legal__layout {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--pb-space-lg);
  }

  .pb-legal__toc {
    position: static;
    padding-bottom: var(--pb-space-sm);
    border-bottom: 1px solid var(--pb-line);
  }

  .pb-legal__toc-nav {
    flex-direction: row;
    overflow-x: auto;
    padding-bottom: 2px;
  }

  .pb-legal__toc-nav a {
    flex-shrink: 0;
  }
}

@media (max-width: 640px) {
  .pb-legal {
    gap: var(--pb-space-xl);
    padding-top: 0;
  }

  .pb-legal__title {
    font-size: clamp(34px, 10vw, 42px);
  }

  .pb-legal__heading {
    font-size: 22px;
  }
}
</style>
