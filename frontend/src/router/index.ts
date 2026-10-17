import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
  type Router,
  type RouterHistory,
} from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AdminLayout from '@/views/admin/AdminLayout.vue'
import FavoritesView from '@/views/FavoritesView.vue'
import HomeView from '@/views/HomeView.vue'
import LegalView from '@/views/LegalView.vue'
import LibraryDetailView from '@/views/LibraryDetailView.vue'
import LibraryListView from '@/views/LibraryListView.vue'
import LibraryQuestionView from '@/views/LibraryQuestionView.vue'
import LoginView from '@/views/LoginView.vue'
import MeView from '@/views/MeView.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import NotificationsView from '@/views/NotificationsView.vue'
import OAuthCallbackView from '@/views/OAuthCallbackView.vue'
import PromptDetailView from '@/views/PromptDetailView.vue'
import PromptFormView from '@/views/PromptFormView.vue'
import RegisterView from '@/views/RegisterView.vue'
import ResetPasswordView from '@/views/ResetPasswordView.vue'
import VerifyEmailView from '@/views/VerifyEmailView.vue'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: HomeView,
  },
  {
    // 隐私政策与用户条款（G-10）。两个页面共用同一视图，文案随界面语言切换。
    path: '/privacy',
    name: 'privacy',
    component: LegalView,
    props: { mode: 'privacy' },
  },
  {
    path: '/terms',
    name: 'terms',
    component: LegalView,
    props: { mode: 'terms' },
  },
  {
    path: '/upload',
    name: 'prompt-upload',
    component: PromptFormView,
    meta: { requiresAuth: true },
  },
  {
    path: '/prompts/:id',
    name: 'prompt-detail',
    component: PromptDetailView,
  },
  {
    path: '/prompts/:id/edit',
    name: 'prompt-edit',
    component: PromptFormView,
    meta: { requiresAuth: true },
  },
  {
    path: '/login',
    name: 'login',
    component: LoginView,
  },
  {
    path: '/register',
    name: 'register',
    component: RegisterView,
  },
  {
    path: '/verify-email',
    name: 'verify-email',
    component: VerifyEmailView,
  },
  {
    path: '/reset-password',
    name: 'reset-password',
    component: ResetPasswordView,
  },
  {
    path: '/oauth/github/callback',
    name: 'oauth-callback',
    component: OAuthCallbackView,
  },
  {
    path: '/me',
    name: 'me',
    component: MeView,
    meta: { requiresAuth: true },
  },
  {
    path: '/favorites',
    name: 'favorites',
    component: FavoritesView,
    meta: { requiresAuth: true },
  },
  {
    path: '/libraries',
    name: 'libraries',
    component: LibraryListView,
    meta: { requiresAuth: true },
  },
  {
    path: '/libraries/:id',
    name: 'library-detail',
    component: LibraryDetailView,
    meta: { requiresAuth: true },
  },
  {
    path: '/library-questions/:id',
    name: 'library-question',
    component: LibraryQuestionView,
    meta: { requiresAuth: true },
  },
  {
    path: '/users/:userId/favorites',
    name: 'user-favorites',
    component: FavoritesView,
  },
  {
    path: '/notifications',
    name: 'notifications',
    component: NotificationsView,
    meta: { requiresAuth: true },
  },
  {
    // 管理后台（需求第 17 章）。各模块按需加载：后台不是常用入口，
    // 静态引入会让首屏包体带上全部管理界面。
    path: '/admin',
    component: AdminLayout,
    meta: { requiresAuth: true, requiresAdmin: true },
    children: [
      { path: '', redirect: '/admin/dashboard' },
      {
        path: 'dashboard',
        name: 'admin-dashboard',
        component: () => import('@/views/admin/AdminDashboardView.vue'),
      },
      {
        path: 'prompts',
        name: 'admin-prompts',
        component: () => import('@/views/admin/AdminPromptsView.vue'),
      },
      {
        path: 'models',
        name: 'admin-models',
        component: () => import('@/views/admin/AdminModelsView.vue'),
      },
      {
        path: 'users',
        name: 'admin-users',
        component: () => import('@/views/admin/AdminUsersView.vue'),
      },
      {
        path: 'tags',
        name: 'admin-tags',
        component: () => import('@/views/admin/AdminTagsView.vue'),
      },
      {
        path: 'dictionary',
        name: 'admin-dictionary',
        component: () => import('@/views/admin/AdminDictionaryView.vue'),
      },
      {
        path: 'reports',
        name: 'admin-reports',
        component: () => import('@/views/admin/AdminReportsView.vue'),
      },
      {
        path: 'invites',
        name: 'admin-invites',
        component: () => import('@/views/admin/AdminInvitesView.vue'),
      },
      {
        path: 'logs',
        name: 'admin-logs',
        component: () => import('@/views/admin/AdminLogsView.vue'),
      },
      {
        path: 'announcements',
        name: 'admin-announcements',
        component: () => import('@/views/admin/AdminAnnouncementsView.vue'),
      },
      {
        path: 'recycle',
        name: 'admin-recycle',
        component: () => import('@/views/admin/AdminRecycleView.vue'),
      },
      {
        path: 'settings',
        name: 'admin-settings',
        component: () => import('@/views/admin/AdminSettingsView.vue'),
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: NotFoundView,
  },
]

/** createAppRouter 构造路由实例，历史实现可替换以便测试。 */
export function createAppRouter(history: RouterHistory = createWebHistory()): Router {
  const router = createRouter({
    history,
    routes,
    scrollBehavior: () => ({ top: 0 }),
  })

  /**
   * 需要登录的页面在进入前先解析登录态；未登录时带 redirect 参数跳到登录页。
   * ensureSession 与应用启动时的探测共享同一次请求，因此守卫不会在
   * 探测尚未返回时就把已登录用户判为游客（开发方案 §3.6）。
   *
   * 后台路由在此再加一道身份判定：非管理员跳到首页。
   * 后端仍会按 admin 中间件独立拦截，前端守卫只避免把无权用户带进空页面。
   */
  router.beforeEach(async (to) => {
    if (to.meta.requiresAuth !== true) {
      return true
    }

    const auth = useAuthStore()
    await auth.ensureSession()

    if (!auth.isAuthenticated) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    if (to.meta.requiresAdmin === true && !auth.hasRole('admin')) {
      return { name: 'home' }
    }
    return true
  })

  return router
}

export const router = createAppRouter()

export default router
