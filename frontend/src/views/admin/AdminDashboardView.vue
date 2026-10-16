// 统计仪表盘（需求 §17.1）。
// 只读页面：展示平台计数与热门、最新提示词榜单，没有写操作入口。

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { adminApi, type Dashboard, type DashboardPrompt } from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()
const { messageOf } = useErrorMessage()

/** CountItem 描述一项计数：label 是 admin.dashboard 下的键名，field 是对应的接口字段名。 */
interface CountItem {
  label: string
  field: string
  value: number
}

/** CountGroup 描述一个计数分组。 */
interface CountGroup {
  title: string
  items: CountItem[]
}

const dashboard = ref<Dashboard | null>(null)
const loading = ref(false)
const failure = ref('')

/** countGroups 按需求 §17.1 的四个分组列出全部 17 项计数。 */
const countGroups = computed<CountGroup[]>(() => {
  const data = dashboard.value
  if (data === null) {
    return []
  }
  return [
    {
      title: 'groupUsers',
      items: [
        { label: 'userTotal', field: 'user_total', value: data.user_total },
        { label: 'userToday', field: 'user_today', value: data.user_today },
        { label: 'userRoleUser', field: 'user_role_user', value: data.user_role_user },
        {
          label: 'userRoleDeveloper',
          field: 'user_role_developer',
          value: data.user_role_developer,
        },
        { label: 'userBanned', field: 'user_banned', value: data.user_banned },
      ],
    },
    {
      title: 'groupPrompts',
      items: [
        { label: 'promptTotal', field: 'prompt_total', value: data.prompt_total },
        { label: 'promptPublic', field: 'prompt_public', value: data.prompt_public },
        { label: 'promptPrivate', field: 'prompt_private', value: data.prompt_private },
        { label: 'promptDeveloper', field: 'prompt_developer', value: data.prompt_developer },
        { label: 'promptToday', field: 'prompt_today', value: data.prompt_today },
      ],
    },
    {
      title: 'groupGovernance',
      items: [
        { label: 'ratingTotal', field: 'rating_total', value: data.rating_total },
        { label: 'commentTotal', field: 'comment_total', value: data.comment_total },
        { label: 'reportPending', field: 'report_pending', value: data.report_pending },
        { label: 'recycleTotal', field: 'recycle_total', value: data.recycle_total },
      ],
    },
    {
      title: 'groupPlatform',
      items: [
        { label: 'modelTotal', field: 'model_total', value: data.model_total },
        { label: 'inviteTotal', field: 'invite_total', value: data.invite_total },
        { label: 'recommendPending', field: 'recommend_pending', value: data.recommend_pending },
      ],
    },
  ]
})

const hotPrompts = computed<DashboardPrompt[]>(() => dashboard.value?.hot_prompts ?? [])
const recentPrompts = computed<DashboardPrompt[]>(() => dashboard.value?.recent_prompts ?? [])

/** dashboardText 返回仪表盘文案，suffix 是 admin.dashboard 下的键名。 */
function dashboardText(suffix: string): string {
  return t(`admin.dashboard.${suffix}`)
}

async function loadDashboard(): Promise<void> {
  loading.value = true
  try {
    dashboard.value = await adminApi.dashboard()
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadDashboard()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title" data-testid="admin-dashboard-title">
        {{ t('admin.dashboard.title') }}
      </h2>
    </header>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-dashboard-error">
      {{ t('admin.common.loadFailed') }}：{{ failure }}
    </p>

    <div class="pb-admin-module__groups">
      <section v-for="group in countGroups" :key="group.title" class="pb-admin-module__group">
        <h3 class="pb-admin-module__group-title">{{ dashboardText(group.title) }}</h3>
        <div class="pb-admin-module__counts">
          <div v-for="item in group.items" :key="item.field" class="pb-admin-module__count">
            <span class="pb-admin-module__count-label">{{ dashboardText(item.label) }}</span>
            <span
              class="pb-admin-module__count-value"
              :data-testid="`admin-dashboard-count-${item.field}`"
            >
              {{ item.value }}
            </span>
          </div>
        </div>
      </section>
    </div>

    <div class="pb-admin-module__lists">
      <section class="pb-admin-module__list" data-testid="admin-dashboard-hot">
        <h3 class="pb-admin-module__group-title">{{ t('admin.dashboard.hotPrompts') }}</h3>
        <p v-if="!loading && hotPrompts.length === 0" class="pb-admin-module__empty">
          {{ t('admin.common.empty') }}
        </p>
        <ul v-else class="pb-admin-module__rank">
          <li v-for="item in hotPrompts" :key="item.id" class="pb-admin-module__rank-item">
            <RouterLink class="pb-admin-module__link" :to="`/prompts/${item.id}`">
              {{ item.title }}
            </RouterLink>
            <span class="pb-admin-module__metric">
              {{ t('prompt.comments') }} {{ item.comment_count }}
            </span>
            <span class="pb-admin-module__metric">
              {{ t('prompt.favorites') }} {{ item.favorite_count }}
            </span>
          </li>
        </ul>
      </section>

      <section class="pb-admin-module__list" data-testid="admin-dashboard-recent">
        <h3 class="pb-admin-module__group-title">{{ t('admin.dashboard.recentPrompts') }}</h3>
        <p v-if="!loading && recentPrompts.length === 0" class="pb-admin-module__empty">
          {{ t('admin.common.empty') }}
        </p>
        <ul v-else class="pb-admin-module__rank">
          <li v-for="item in recentPrompts" :key="item.id" class="pb-admin-module__rank-item">
            <RouterLink class="pb-admin-module__link" :to="`/prompts/${item.id}`">
              {{ item.title }}
            </RouterLink>
            <span class="pb-admin-module__metric">
              {{ t('prompt.comments') }} {{ item.comment_count }}
            </span>
            <span class="pb-admin-module__metric">
              {{ t('prompt.favorites') }} {{ item.favorite_count }}
            </span>
          </li>
        </ul>
      </section>
    </div>
  </section>
</template>

<style scoped>
.pb-admin-module__head {
  align-items: baseline;
  display: flex;
  gap: var(--pb-space-md);
  justify-content: space-between;
}

.pb-admin-module__title {
  font-size: 18px;
  margin: 0;
}

.pb-admin-module__error {
  color: var(--el-color-danger);
}

.pb-admin-module__empty {
  color: var(--pb-color-text-soft);
}

.pb-admin-module__groups {
  display: grid;
  gap: var(--pb-space-md);
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  margin: var(--pb-space-sm) 0;
}

.pb-admin-module__group,
.pb-admin-module__list {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 4px;
  padding: var(--pb-space-sm) var(--pb-space-md);
}

.pb-admin-module__group-title {
  font-size: 15px;
  margin: 0 0 var(--pb-space-sm);
}

.pb-admin-module__counts {
  display: grid;
  gap: var(--pb-space-xs) var(--pb-space-md);
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
}

.pb-admin-module__count {
  display: flex;
  gap: var(--pb-space-xs);
  justify-content: space-between;
}

.pb-admin-module__count-label {
  color: var(--pb-color-text-soft);
}

.pb-admin-module__count-value {
  font-weight: 600;
}

.pb-admin-module__lists {
  display: grid;
  gap: var(--pb-space-md);
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
}

.pb-admin-module__rank {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-admin-module__rank-item {
  align-items: baseline;
  border-bottom: 1px solid var(--el-border-color-lighter);
  display: flex;
  gap: var(--pb-space-sm);
  justify-content: space-between;
  padding: var(--pb-space-xs) 0;
}

.pb-admin-module__link {
  color: var(--el-color-primary);
  text-decoration: none;
}

.pb-admin-module__metric {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  white-space: nowrap;
}
</style>
