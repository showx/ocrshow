import { createRouter, createWebHistory } from 'vue-router'
import { ensureUser } from './auth'
import HomeView from './views/HomeView.vue'
import JobView from './views/JobView.vue'
import LoginView from './views/LoginView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
    { path: '/', name: 'home', component: HomeView },
    { path: '/jobs/:id', name: 'job', component: JobView, props: true },
  ],
})

router.beforeEach(async (to) => {
  const user = await ensureUser()
  if (to.meta.public) {
    if (to.name === 'login' && user) {
      const redirect = typeof to.query.redirect === 'string' ? to.query.redirect : '/'
      return redirect || '/'
    }
    return true
  }
  if (!user) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  return true
})

export default router
