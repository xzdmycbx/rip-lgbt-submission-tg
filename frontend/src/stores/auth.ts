import { defineStore } from 'pinia';
import { authAPI, type AdminInfo } from '@/api/client';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    admin: null as AdminInfo | null,
    loading: false
  }),
  actions: {
    async refresh() {
      this.loading = true;
      try {
        const r = await authAPI.me();
        this.admin = r.admin;
      } finally {
        this.loading = false;
      }
    },
    async logout() {
      await authAPI.logout();
      this.admin = null;
    }
  }
});
