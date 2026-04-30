<template>
  <main class="placeholder">
    <p style="color: var(--muted)">预览页面 #{{ draftId }} (内部使用，由 chromedp 抓取)。</p>
    <p>{{ draft?.display_name || '草稿' }}</p>
    <article v-if="profile">
      <h1 style="font-family: var(--serif);">{{ profile.name }}</h1>
      <p>{{ profile.desc }}</p>
      <div class="story" v-if="profile.contentHtml" v-html="profile.contentHtml"></div>
    </article>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { api } from '@/api/client';

const props = defineProps<{ draftId: string }>();
const draft = ref<any>(null);
const profile = ref<any>(null);

onMounted(async () => {
  try {
    const r = await api.get(`/api/admin/drafts/${encodeURIComponent(props.draftId)}/preview`);
    draft.value = r.data.draft;
    profile.value = r.data.profile;
  } catch {
    /* 草稿可能尚不可用 */
  }
});
</script>
