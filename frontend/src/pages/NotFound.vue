<template>
  <SiteHeader />
  <main>
    <section class="placeholder">
      <p class="eyebrow">404</p>
      <h1>这里没有找到名字</h1>
      <p class="lede">可能是链接已经变化，或这位逝者暂未收录。</p>
      <RouterLink class="button primary" to="/">回到索引</RouterLink>
    </section>
  </main>
  <SiteFooter />
</template>

<script setup lang="ts">
import SiteHeader from '@/components/SiteHeader.vue';
import SiteFooter from '@/components/SiteFooter.vue';
</script>
