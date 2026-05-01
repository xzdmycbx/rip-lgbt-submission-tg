import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'home', component: () => import('@/pages/Home.vue') },
  { path: '/memorial/:id', name: 'memorial', component: () => import('@/pages/MemorialDetail.vue'), props: true },
  { path: '/submit', name: 'submit', component: () => import('@/pages/Submit.vue') },
  { path: '/admin', redirect: '/admin/queue' },
  { path: '/admin/login', name: 'admin-login', component: () => import('@/pages/admin/Login.vue') },
  { path: '/admin/login/totp', name: 'admin-login-totp', component: () => import('@/pages/admin/LoginTOTP.vue') },
  { path: '/admin/setup-2fa', name: 'admin-setup-2fa', component: () => import('@/pages/admin/Setup2FA.vue') },
  { path: '/admin/queue', name: 'admin-queue', component: () => import('@/pages/admin/Queue.vue') },
  { path: '/admin/review/:draftId', name: 'admin-review', component: () => import('@/pages/admin/Review.vue'), props: true },
  { path: '/admin/memorials', name: 'admin-memorials', component: () => import('@/pages/admin/Memorials.vue') },
  { path: '/admin/memorials/:id', name: 'admin-memorial-edit', component: () => import('@/pages/admin/MemorialEdit.vue'), props: true },
  { path: '/admin/profile', name: 'admin-profile', component: () => import('@/pages/admin/Profile.vue') },
  { path: '/admin/admins', name: 'admin-admins', component: () => import('@/pages/admin/Admins.vue') },
  { path: '/admin/settings', name: 'admin-settings', component: () => import('@/pages/admin/Settings.vue') },
  { path: '/admin/preview/:draftId', name: 'admin-preview', component: () => import('@/pages/admin/Preview.vue'), props: true },
  { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('@/pages/NotFound.vue') }
];

export const router = createRouter({
  history: createWebHistory(),
  routes
});
