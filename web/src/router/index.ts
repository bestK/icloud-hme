import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  { path: '/login', component: () => import('@/views/LoginView.vue'), meta: { public: true } },
  { path: '/', redirect: () => (useAuthStore().isAdmin ? '/overview' : '/aliases') },
  { path: '/overview', component: () => import('@/views/OverviewView.vue'), meta: { admin: true } },
  { path: '/tokens', component: () => import('@/views/TokensView.vue'), meta: { admin: true } },
  { path: '/aliases', component: () => import('@/views/AliasesView.vue') },
  { path: '/inbox', component: () => import('@/views/InboxView.vue') },
  { path: '/:pathMatch(.*)*', component: () => import('@/views/NotFoundView.vue'), meta: { public: true } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.meta.public) return true
  if (!auth.isLoggedIn) return { path: '/login', query: { r: to.fullPath } }
  if (to.meta.admin && !auth.isAdmin) return { path: '/aliases' }
  return true
})

export default router
