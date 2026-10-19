<script setup lang="ts">
// 隐私政策与用户条款（G-10）。静态页面，文案来自语言包，随界面语言切换。
// 两个路由共用本视图，由 mode 决定渲染哪一份文本。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DocToc from '@/components/DocToc.vue'

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
        id: 'scope',
        title: t('privacy.scopeTitle'),
        body: paragraphs('privacy.scopeBody'),
      },
      {
        id: 'collect',
        title: t('privacy.collectTitle'),
        body: paragraphs('privacy.collectBody'),
      },
      {
        id: 'basis',
        title: t('privacy.basisTitle'),
        body: paragraphs('privacy.basisBody'),
      },
      { id: 'use', title: t('privacy.useTitle'), body: paragraphs('privacy.useBody') },
      {
        id: 'storage',
        title: t('privacy.storageTitle'),
        body: paragraphs('privacy.storageBody'),
      },
      { id: 'share', title: t('privacy.shareTitle'), body: paragraphs('privacy.shareBody') },
      {
        id: 'transfer',
        title: t('privacy.transferTitle'),
        body: paragraphs('privacy.transferBody'),
      },
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
      {
        id: 'automated',
        title: t('privacy.automatedTitle'),
        body: paragraphs('privacy.automatedBody'),
      },
      { id: 'minor', title: t('privacy.minorTitle'), body: paragraphs('privacy.minorBody') },
      {
        id: 'update',
        title: t('privacy.updateTitle'),
        body: paragraphs('privacy.updateBody'),
      },
      {
        id: 'contact',
        title: t('privacy.contactTitle'),
        body: paragraphs('privacy.contactBody'),
      },
    ]
  }
  return [
    {
      id: 'service',
      title: t('terms.serviceTitle'),
      body: paragraphs('terms.serviceBody'),
    },
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
      id: 'availability',
      title: t('terms.availabilityTitle'),
      body: paragraphs('terms.availabilityBody'),
    },
    {
      id: 'termination',
      title: t('terms.terminationTitle'),
      body: paragraphs('terms.terminationBody'),
    },
    { id: 'liability', title: t('terms.liabilityTitle'), body: paragraphs('terms.liabilityBody') },
    { id: 'indemnity', title: t('terms.indemnityTitle'), body: paragraphs('terms.indemnityBody') },
    { id: 'dispute', title: t('terms.disputeTitle'), body: paragraphs('terms.disputeBody') },
    { id: 'update', title: t('terms.updateTitle'), body: paragraphs('terms.updateBody') },
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
      <p class="pb-legal__meta" data-testid="legal-meta">
        <span class="pb-legal__updated">{{ t('legal.updated') }}</span>
        <span class="pb-legal__dot" aria-hidden="true">·</span>
        <span>{{ t('legal.effective') }}</span>
        <span class="pb-legal__dot" aria-hidden="true">·</span>
        <span>{{ t('legal.version') }}</span>
      </p>
      <p class="pb-legal__operator" data-testid="legal-operator">{{ t('legal.operatorNote') }}</p>
      <p class="pb-legal__intro">{{ intro }}</p>
    </header>

    <div class="pb-legal__layout">
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

    <DocToc
      :sections="sections"
      :label="t('methodology.toc')"
      :close-label="t('methodology.tocClose')"
    />
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

.pb-legal__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin: 0;
  color: var(--pb-ink-faint);
  font-size: 10.5px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.pb-legal__updated {
  color: inherit;
}

.pb-legal__dot {
  opacity: 0.5;
}

.pb-legal__operator {
  max-width: 72ch;
  margin: var(--pb-space-sm) 0 0;
  padding: 12px 16px;
  border-left: 2px solid var(--pb-accent);
  background: var(--pb-surface-sunken);
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  line-height: 1.8;
}

.pb-legal__intro {
  max-width: 72ch;
  margin: var(--pb-space-sm) 0 0;
  color: var(--pb-ink-soft);
  font-size: 15px;
  line-height: 1.85;
}

/* 目录改为右下角的浮动窗口，正文不再为侧栏预留列宽。 */
.pb-legal__layout {
  display: block;
}

.pb-legal__content {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xl);
  max-width: 760px;
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
  /* 窄屏顶栏为两行，直接打开带片段的地址时仍要让标题落在顶栏下方。 */
  .pb-legal__section {
    scroll-margin-top: 120px;
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
