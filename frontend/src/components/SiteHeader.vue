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
    </nav>
  </header>
</template>

<script setup lang="ts"></script>
