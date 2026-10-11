import {
  createRouter,
  createWebHistory,
  type RouteRecordRaw,
  type Router,
  type RouterHistory,
} from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import HomeView from '@/views/HomeView.vue'
import LoginView from '@/views/LoginView.vue'
import MeView from '@/views/MeView.vue'
import NotFoundView from '@/views/NotFoundView.vue'
import OAuthCallbackView from '@/views/OAuthCallbackView.vue'
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
   * 需要登录的页面在进入前先探测登录态；未登录时带 redirect 参数跳到登录页。
   * 探测请求本身标记 silent，因此游客不会因 401 被反复弹回。
   */
  router.beforeEach(async (to) => {
    if (to.meta.requiresAuth !== true) {
      return true
    }

    const auth = useAuthStore()
    if (auth.status === 'unknown') {
      await auth.fetchSession()
    }
    if (!auth.isAuthenticated) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    return true
  })

  return router
}

export const router = createAppRouter()

export default router
