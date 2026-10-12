import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
  type Router,
  type RouterHistory,
} from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import FavoritesView from '@/views/FavoritesView.vue'
import HomeView from '@/views/HomeView.vue'
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
    return true
  })

  return router
}

export const router = createAppRouter()

export default router
