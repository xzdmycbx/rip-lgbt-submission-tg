<template>
  <SiteHeader />
  <main>
    <section style="padding: 4rem 0">
      <p class="eyebrow">SUBMISSION</p>
      <h1>提交纪念条目</h1>
      <p class="lede">
        投稿现已迁移到 Telegram 机器人。请在 Telegram 私聊 <strong>@rip_lgbt_bot</strong>（或站点维护者公布的机器人账号）发送 <code>/submit</code>，机器人会一步步引导你填写。
      </p>
      <p class="lede">
        管理员收到投稿后会预览实际页面效果，必要时机器人会请你修改某一节，最后审核通过即在网站上线。
      </p>
      <div class="hero-actions" style="margin-top: 2rem;">
        <RouterLink class="button primary" to="/">返回索引</RouterLink>
      </div>
    </section>
  </main>
  <SiteFooter />
</template>

<script setup lang="ts">
import SiteHeader from '@/components/SiteHeader.vue';
import SiteFooter from '@/components/SiteFooter.vue';
</script>
