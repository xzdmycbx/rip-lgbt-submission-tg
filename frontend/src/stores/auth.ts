import { defineStore } from 'pinia';
import { authAPI, type AdminInfo } from '@/api/client';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    admin: null as AdminInfo | null,
    loading: false,
    fetched: false
  }),
  actions: {
    async refresh(force = false) {
      if (!force && this.fetched) return this.admin;
      this.loading = true;
      try {
        const r = await authAPI.me();
        this.admin = r.admin;
        this.fetched = true;
      } finally {
        this.loading = false;
      }
      return this.admin;
    },
    async logout() {
      await authAPI.logout();
      this.admin = null;
      this.fetched = true;
    }
  }
});
