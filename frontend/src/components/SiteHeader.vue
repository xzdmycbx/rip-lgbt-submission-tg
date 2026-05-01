<template>
  <header class="site-header">
    <RouterLink class="brand" to="/" aria-label="勿忘我">
      <span class="brand-mark" aria-hidden="true">勿</span>
      <span>
        <strong>勿忘我</strong>
        <small>rip.lgbt</small>
      </span>
    </RouterLink>
    <nav class="header-links" aria-label="页面导航">
      <RouterLink to="/submit">投稿</RouterLink>
      <slot />
      <button
        type="button"
        class="theme-toggle"
        :aria-label="theme.theme === 'dark' ? '切换到亮色模式' : '切换到暗色模式'"
        :title="theme.theme === 'dark' ? '亮色模式' : '暗色模式'"
        @click="theme.toggle()"
      >
        <span aria-hidden="true">{{ theme.theme === 'dark' ? '☀' : '☾' }}</span>
      </button>
    </nav>
  </header>
</template>

<script setup lang="ts">
import { useThemeStore } from '@/stores/theme';

const theme = useThemeStore();
</script>

<style scoped>
.theme-toggle {
  display: inline-grid;
  place-items: center;
  width: 2.1rem;
  height: 2.1rem;
  margin-left: .15rem;
  border: 1px solid var(--line);
  border-radius: 999px;
  background: var(--input-bg);
  color: var(--ink);
  font-size: 1rem;
  cursor: pointer;
  transition: background .15s ease, border-color .15s ease, transform .15s ease;
}

.theme-toggle:hover {
  border-color: var(--line-strong);
  transform: translateY(-1px);
}

.theme-toggle:focus-visible {
  outline: 2px solid var(--blue);
  outline-offset: 2px;
}
</style>
