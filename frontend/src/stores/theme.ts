import { defineStore } from 'pinia';

export type Theme = 'dark' | 'light';

const STORAGE_KEY = 'rip-theme';

function readSavedTheme(): Theme | null {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'dark' || v === 'light') return v;
  } catch {
    // localStorage may be disabled (private mode); fall through.
  }
  return null;
}

function detectInitial(): Theme {
  const saved = readSavedTheme();
  if (saved) return saved;
  if (typeof window !== 'undefined' && window.matchMedia) {
    return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark';
  }
  return 'dark';
}

export const useThemeStore = defineStore('theme', {
  state: () => ({ theme: detectInitial() as Theme }),
  actions: {
    apply() {
      if (typeof document === 'undefined') return;
      document.documentElement.setAttribute('data-theme', this.theme);
    },
    set(t: Theme) {
      this.theme = t;
      try {
        localStorage.setItem(STORAGE_KEY, t);
      } catch {
        // ignore storage errors
      }
      this.apply();
    },
    toggle() {
      this.set(this.theme === 'dark' ? 'light' : 'dark');
    }
  }
});
