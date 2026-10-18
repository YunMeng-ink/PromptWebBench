<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DocToc from '@/components/DocToc.vue'

const { t, tm } = useI18n()

const SECTIONS = [
  { id: 'scope', titleKey: 'methodology.scopeTitle', bodyKey: 'methodology.scopeBody' },
  { id: 'rating', titleKey: 'methodology.ratingTitle', bodyKey: 'methodology.ratingBody' },
  { id: 'windows', titleKey: 'methodology.windowTitle', bodyKey: 'methodology.windowBody' },
  { id: 'adjusted', titleKey: 'methodology.adjustedTitle', bodyKey: 'methodology.adjustedBody' },
  {
    id: 'discrimination',
    titleKey: 'methodology.discriminationTitle',
    bodyKey: 'methodology.discriminationBody',
  },
  {
    id: 'confidence',
    titleKey: 'methodology.confidenceTitle',
    bodyKey: 'methodology.confidenceBody',
  },
  { id: 'labels', titleKey: 'methodology.labelsTitle', bodyKey: 'methodology.labelsBody' },
  {
    id: 'interpretation',
    titleKey: 'methodology.interpretTitle',
    bodyKey: 'methodology.interpretBody',
  },
  {
    id: 'parameters',
    titleKey: 'methodology.parametersTitle',
    bodyKey: 'methodology.parametersBody',
  },
] as const

/** paragraphs 读取键下的段落数组；语言包缺失时返回空数组。 */
function paragraphs(key: string): string[] {
  const value = tm(key)
  if (!Array.isArray(value)) {
    return []
  }
  return value.filter((item): item is string => typeof item === 'string')
}

const sections = computed(() =>
  SECTIONS.map((section, index) => ({
    id: section.id,
    index: String(index + 1).padStart(2, '0'),
    title: t(section.titleKey),
    body: paragraphs(section.bodyKey),
  })),
)

const scores = computed(() =>
  [1, 2, 3, 4, 5].map((score) => ({
    score,
    label: t(`prompt.score${score}`),
    anchor: t(`methodology.ratingAnchor${score}`),
  })),
)

const parameters = computed(() => [
  { key: 'bayes_prior', value: '3', meaning: t('methodology.paramBayesPrior') },
  { key: 'bayes_k', value: '5', meaning: t('methodology.paramBayesK') },
  { key: 'discrim_min_models', value: '3', meaning: t('methodology.paramMinModels') },
  {
    key: 'discrim_min_ratings_per_model',
    value: '3',
    meaning: t('methodology.paramMinRatingsPerModel'),
  },
  {
    key: 'discrim_min_total_ratings',
    value: '10',
    meaning: t('methodology.paramMinTotalRatings'),
  },
  {
    key: 'discrim_model_count_target',
    value: '4',
    meaning: t('methodology.paramModelTarget'),
  },
  {
    key: 'discrim_rating_count_target',
    value: '20',
    meaning: t('methodology.paramRatingTarget'),
  },
  {
    key: 'saturation_threshold',
    value: '4.5',
    meaning: t('methodology.paramSaturation'),
  },
  {
    key: 'obsolete_discrimination_threshold',
    value: '2.0',
    meaning: t('methodology.paramObsolete'),
  },
  { key: 'bootstrap_iterations', value: '500', meaning: t('methodology.paramBootstrap') },
  { key: 'recent_window_days', value: '7', meaning: t('methodology.paramRecentWindow') },
])

const exampleModels = [
  { name: 'Model A', adjusted: '1.4' },
  { name: 'Model B', adjusted: '2.6' },
  { name: 'Model C', adjusted: '4.3' },
  { name: 'Model D', adjusted: '5.0' },
]
</script>

<template>
  <article class="pb-doc" data-testid="methodology-page">
    <header class="pb-doc__hero">
      <span class="pb-eyebrow" aria-hidden="true">Measurement / Methodology</span>
      <h1 class="pb-doc__title">{{ t('methodology.title') }}</h1>
      <p class="pb-doc__lead">{{ t('methodology.lead') }}</p>
      <dl class="pb-doc__meta">
        <div>
          <dt>{{ t('methodology.updatedLabel') }}</dt>
          <dd>{{ t('methodology.updated') }}</dd>
        </div>
        <div>
          <dt>{{ t('methodology.scopeChip') }}</dt>
          <dd>{{ t('methodology.scopeValue') }}</dd>
        </div>
        <div>
          <dt>{{ t('methodology.parameterLabel') }}</dt>
          <dd>{{ t('methodology.parameterValue') }}</dd>
        </div>
      </dl>
    </header>

    <div class="pb-doc__layout">
      <div class="pb-doc__content">
        <section
          v-for="section in sections"
          :id="section.id"
          :key="section.id"
          class="pb-doc__section"
        >
          <h2 class="pb-doc__heading">
            <span class="pb-doc__index" aria-hidden="true">{{ section.index }}</span>
            {{ section.title }}
          </h2>
          <p v-for="line in section.body" :key="line" class="pb-doc__paragraph">{{ line }}</p>

          <div v-if="section.id === 'rating'" class="pb-doc__table-wrap">
            <table class="pb-doc__table">
              <thead>
                <tr>
                  <th scope="col">{{ t('methodology.ratingTableHeadScore') }}</th>
                  <th scope="col">{{ t('methodology.ratingTableHeadMeaning') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in scores" :key="row.score">
                  <td class="pb-doc__score">
                    <span class="pb-doc__score-value">{{ row.score }}</span>
                    <span class="pb-doc__score-label">{{ row.label }}</span>
                  </td>
                  <td>{{ row.anchor }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="section.id === 'windows'" class="pb-doc__cards">
            <div class="pb-doc__card">
              <h3>{{ t('methodology.historyWindowTitle') }}</h3>
              <p>{{ t('methodology.historyWindowBody') }}</p>
            </div>
            <div class="pb-doc__card">
              <h3>{{ t('methodology.recentWindowTitle') }}</h3>
              <p>{{ t('methodology.recentWindowBody') }}</p>
            </div>
          </div>

          <div v-if="section.id === 'adjusted'" class="pb-doc__formula">
            <span class="pb-doc__formula-label">{{ t('methodology.adjustedFormulaLabel') }}</span>
            <code>adjusted = (n × mean + k × prior) ÷ (n + k)</code>
          </div>

          <div v-if="section.id === 'adjusted'" class="pb-doc__callout">
            <h3>{{ t('methodology.adjustedExampleTitle') }}</h3>
            <p>{{ t('methodology.adjustedExampleBody') }}</p>
            <code class="pb-doc__inline-code">{{ t('methodology.adjustedExampleResult') }}</code>
          </div>

          <div v-if="section.id === 'discrimination'" class="pb-doc__thresholds">
            <h3>{{ t('methodology.thresholdTitle') }}</h3>
            <ul>
              <li>{{ t('methodology.threshold1') }}</li>
              <li>{{ t('methodology.threshold2') }}</li>
              <li>{{ t('methodology.threshold3') }}</li>
            </ul>
          </div>

          <div v-if="section.id === 'discrimination'" class="pb-doc__formula">
            <span class="pb-doc__formula-label">{{ t('methodology.discriminationFormulaLabel') }}</span>
            <code>range = max(adjusted) − min(adjusted)</code>
            <code>data_coefficient = min(1, valid_models ÷ 4) × min(1, ratings ÷ 20)</code>
            <code>discrimination = clamp(1 + range × data_coefficient, 1, 5)</code>
          </div>

          <div v-if="section.id === 'discrimination'" class="pb-doc__callout">
            <h3>{{ t('methodology.exampleTitle') }}</h3>
            <p>{{ t('methodology.exampleIntro') }}</p>
            <div class="pb-doc__table-wrap">
              <table class="pb-doc__table pb-doc__table--compact">
                <thead>
                  <tr>
                    <th scope="col">{{ t('methodology.exampleModel') }}</th>
                    <th scope="col">{{ t('methodology.exampleAdjusted') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="model in exampleModels" :key="model.name">
                    <td>{{ model.name }}</td>
                    <td class="pb-doc__numeric">{{ model.adjusted }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <code class="pb-doc__inline-code">{{ t('methodology.exampleResult') }}</code>
          </div>

          <div v-if="section.id === 'confidence'" class="pb-doc__formula">
            <span class="pb-doc__formula-label">{{ t('methodology.completionConfidenceLabel') }}</span>
            <code>confidence = 0.4 × min(1, n ÷ 20)</code>
            <code>           + 0.3 × min(1, u ÷ 5)</code>
            <code>           + 0.3 × max(0, 1 − h ÷ 1.0)</code>
          </div>

          <div v-if="section.id === 'confidence'" class="pb-doc__table-wrap">
            <table class="pb-doc__table">
              <thead>
                <tr>
                  <th scope="col">{{ t('methodology.confidenceLevel') }}</th>
                  <th scope="col">{{ t('methodology.confidenceCondition') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>{{ t('prompt.confidenceHigh') }}</td>
                  <td>{{ t('methodology.completionHighCondition') }}</td>
                </tr>
                <tr>
                  <td>{{ t('prompt.confidenceMedium') }}</td>
                  <td>{{ t('methodology.completionMediumCondition') }}</td>
                </tr>
                <tr>
                  <td>{{ t('prompt.confidenceLow') }}</td>
                  <td>{{ t('methodology.completionLowCondition') }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="section.id === 'confidence'" class="pb-doc__formula">
            <span class="pb-doc__formula-label">{{ t('methodology.discriminationConfidenceLabel') }}</span>
            <code>Bootstrap × 500</code>
            <code>95% CI = [2.5%, 97.5%] percentile</code>
          </div>

          <div v-if="section.id === 'confidence'" class="pb-doc__table-wrap">
            <table class="pb-doc__table">
              <thead>
                <tr>
                  <th scope="col">{{ t('methodology.confidenceLevel') }}</th>
                  <th scope="col">{{ t('methodology.confidenceCondition') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td>{{ t('prompt.confidenceHigh') }}</td>
                  <td>{{ t('methodology.discriminationHighCondition') }}</td>
                </tr>
                <tr>
                  <td>{{ t('prompt.confidenceMedium') }}</td>
                  <td>{{ t('methodology.discriminationMediumCondition') }}</td>
                </tr>
                <tr>
                  <td>{{ t('prompt.confidenceLow') }}</td>
                  <td>{{ t('methodology.discriminationLowCondition') }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="section.id === 'labels'" class="pb-doc__labels">
            <div class="pb-doc__label-item">
              <h3>{{ t('methodology.labelObsoleteTitle') }}</h3>
              <p>{{ t('methodology.labelObsoleteBody') }}</p>
            </div>
            <div class="pb-doc__label-item">
              <h3>{{ t('methodology.labelSuspectedObsoleteTitle') }}</h3>
              <p>{{ t('methodology.labelSuspectedObsoleteBody') }}</p>
            </div>
            <div class="pb-doc__label-item">
              <h3>{{ t('methodology.labelSaturationTitle') }}</h3>
              <p>{{ t('methodology.labelSaturationBody') }}</p>
            </div>
            <div class="pb-doc__label-item">
              <h3>{{ t('methodology.labelSuspectedSaturationTitle') }}</h3>
              <p>{{ t('methodology.labelSuspectedSaturationBody') }}</p>
            </div>
          </div>

          <div v-if="section.id === 'parameters'" class="pb-doc__table-wrap">
            <table class="pb-doc__table">
              <thead>
                <tr>
                  <th scope="col">{{ t('methodology.configKey') }}</th>
                  <th scope="col">{{ t('methodology.configValue') }}</th>
                  <th scope="col">{{ t('methodology.configMeaning') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in parameters" :key="item.key">
                  <td class="pb-doc__config-key">{{ item.key }}</td>
                  <td class="pb-doc__numeric">{{ item.value }}</td>
                  <td>{{ item.meaning }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <p v-if="sections.length > 0" class="pb-doc__note">{{ t('methodology.parametersNote') }}</p>

        <RouterLink class="pb-doc__back" to="/">
          {{ t('methodology.back') }}
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
.pb-doc {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-2xl);
  padding: var(--pb-space-xl) 0 var(--pb-space-3xl);
}

.pb-doc__hero {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  padding-bottom: var(--pb-space-xl);
  border-bottom: 1px solid var(--pb-line);
}

.pb-doc__hero::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 180px;
  height: 2px;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
}

.pb-doc__title {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: clamp(38px, 5vw, 54px);
  font-weight: 500;
  letter-spacing: -0.02em;
  line-height: 1.1;
}

.pb-doc__lead {
  max-width: 62ch;
  margin: var(--pb-space-xs) 0 0;
  color: var(--pb-ink-soft);
  font-size: 15.5px;
  line-height: 1.8;
}

.pb-doc__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xl);
  margin: var(--pb-space-md) 0 0;
}

.pb-doc__meta dt {
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.pb-doc__meta dd {
  margin: 4px 0 0;
  color: var(--pb-ink);
  font-size: 13px;
}

/* 目录改为右下角的浮动窗口，正文不再为侧栏预留列宽。 */
.pb-doc__layout {
  display: block;
}

.pb-doc__content {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xl);
  max-width: 780px;
  min-width: 0;
}

.pb-doc__section {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  min-width: 0;
  scroll-margin-top: calc(var(--pb-header-height) + var(--pb-space-lg));
}

.pb-doc__heading {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin: 0 0 var(--pb-space-xs);
  font-family: var(--pb-font-display);
  font-size: 27px;
  font-weight: 500;
  letter-spacing: -0.012em;
  line-height: 1.25;
}

.pb-doc__index {
  color: var(--pb-accent);
  font-family: var(--pb-font-mono);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.06em;
}

.pb-doc__paragraph {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 14.5px;
  line-height: 1.9;
}

.pb-doc__table-wrap {
  overflow-x: auto;
  margin-top: var(--pb-space-xs);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
}

.pb-doc__table {
  width: 100%;
  min-width: 520px;
  border-collapse: collapse;
  font-size: 13px;
  line-height: 1.7;
  font-variant-numeric: tabular-nums;
}

.pb-doc__table th,
.pb-doc__table td {
  padding: 10px 14px;
  border-bottom: 1px solid var(--pb-line);
  text-align: left;
  vertical-align: top;
}

.pb-doc__table th {
  background: var(--pb-surface-muted);
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  white-space: nowrap;
}

.pb-doc__table tbody tr:last-child td {
  border-bottom: 0;
}

.pb-doc__table tbody tr:hover {
  background: color-mix(in srgb, var(--pb-surface-muted) 52%, transparent);
}

.pb-doc__table--compact {
  min-width: 0;
}

.pb-doc__score {
  display: flex;
  align-items: baseline;
  gap: 9px;
  white-space: nowrap;
}

.pb-doc__score-value {
  color: var(--pb-accent);
  font-family: var(--pb-font-display);
  font-size: 18px;
}

.pb-doc__score-label {
  color: var(--pb-ink);
  font-weight: 500;
}

.pb-doc__numeric {
  font-family: var(--pb-font-mono);
  font-size: 12.5px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.pb-doc__config-key {
  color: var(--pb-ink);
  font-family: var(--pb-font-mono);
  font-size: 12px;
  white-space: nowrap;
}

.pb-doc__cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--pb-space-sm);
  margin-top: var(--pb-space-xs);
}

.pb-doc__card {
  padding: var(--pb-space-md);
  background: color-mix(in srgb, var(--pb-surface-muted) 58%, transparent);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
}

.pb-doc__card h3 {
  margin: 0 0 5px;
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.pb-doc__card p {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13px;
  line-height: 1.75;
}

.pb-doc__formula {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-top: var(--pb-space-xs);
  padding: var(--pb-space-md) var(--pb-space-lg);
  overflow-x: auto;
  background: var(--pb-surface-sunken);
  border: 1px solid var(--pb-line);
  border-left: 3px solid var(--pb-purpose);
  border-radius: 0 var(--pb-radius-md) var(--pb-radius-md) 0;
}

.pb-doc__formula-label {
  color: var(--pb-ink-faint);
  font-size: 9.5px;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.pb-doc__formula code {
  color: var(--pb-ink);
  font-family: var(--pb-font-mono);
  font-size: 13px;
  line-height: 1.8;
  white-space: pre;
}

.pb-doc__callout {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  margin-top: var(--pb-space-xs);
  padding: var(--pb-space-lg);
  background:
    linear-gradient(135deg, var(--pb-accent-soft), transparent 70%),
    var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
}

.pb-doc__callout h3 {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: 19px;
  font-weight: 500;
}

.pb-doc__callout p {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  line-height: 1.8;
}

.pb-doc__inline-code {
  align-self: flex-start;
  max-width: 100%;
  padding: 6px 10px;
  overflow-x: auto;
  border-radius: var(--pb-radius-sm);
  background: var(--pb-surface-sunken);
  color: var(--pb-ink);
  font-family: var(--pb-font-mono);
  font-size: 12.5px;
  white-space: nowrap;
}

.pb-doc__thresholds {
  padding: var(--pb-space-md) var(--pb-space-lg);
  background: var(--pb-surface);
  border: 1px dashed var(--pb-line-strong);
  border-radius: var(--pb-radius-md);
}

.pb-doc__thresholds h3 {
  margin: 0 0 6px;
  font-size: 13px;
  font-weight: 600;
}

.pb-doc__thresholds ul {
  margin: 0;
  padding-left: 18px;
  color: var(--pb-ink-soft);
  font-size: 13px;
  line-height: 1.85;
}

.pb-doc__labels {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--pb-space-sm);
  margin-top: var(--pb-space-xs);
}

.pb-doc__label-item {
  padding: var(--pb-space-md);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-top: 2px solid var(--pb-group-reliability);
  border-radius: var(--pb-radius-md);
}

.pb-doc__label-item h3 {
  margin: 0 0 5px;
  font-size: 13.5px;
  font-weight: 600;
}

.pb-doc__label-item p {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 12.5px;
  line-height: 1.75;
}

.pb-doc__note {
  margin: 0;
  padding-top: var(--pb-space-md);
  border-top: 1px solid var(--pb-line);
  color: var(--pb-ink-faint);
  font-size: 12px;
  line-height: 1.8;
}

.pb-doc__back {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  width: fit-content;
  color: var(--pb-accent);
  font-size: 13.5px;
}

.pb-doc__back::before {
  content: "";
  width: 16px;
  height: 1px;
  background: currentColor;
  opacity: 0.6;
}

.pb-doc__back:hover {
  color: var(--pb-accent-strong);
}

@media (max-width: 900px) {
  /* 窄屏顶栏为两行，直接打开带片段的地址时仍要让标题落在顶栏下方。 */
  .pb-doc__section {
    scroll-margin-top: 120px;
  }
}

@media (max-width: 640px) {
  .pb-doc {
    gap: var(--pb-space-xl);
    padding-top: 0;
  }

  .pb-doc__title {
    font-size: clamp(34px, 10vw, 42px);
  }

  .pb-doc__meta {
    gap: var(--pb-space-md);
  }

  .pb-doc__heading {
    font-size: 23px;
  }

  .pb-doc__formula {
    padding: var(--pb-space-md);
  }

  .pb-doc__callout {
    padding: var(--pb-space-md);
  }

  .pb-doc__table {
    min-width: 480px;
  }
}
</style>
