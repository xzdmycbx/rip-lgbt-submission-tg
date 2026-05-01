<template>
  <main class="preview-wrap">
    <article class="profile" v-if="profile">
      <header class="profile-hero">
        <div class="profile-photo">
          <img v-if="profile.profileUrl" class="profile-avatar" :src="profile.profileUrl" alt="" />
          <div v-else class="profile-mark" aria-hidden="true">{{ firstGlyph(profile.name) }}</div>
        </div>
        <div>
          <p class="eyebrow">PREVIEW · 投稿预览</p>
          <h1>{{ profile.name || '(未填写展示名)' }}</h1>
          <p class="profile-date">{{ formatDate(profile.departure) }}</p>
          <p class="profile-desc">{{ profile.desc || '尚未填写一句话简介。' }}</p>
        </div>
      </header>

      <section class="profile-section" v-if="profile.facts.length">
        <h2>公开信息</h2>
        <dl class="facts">
          <template v-for="fact in profile.facts" :key="fact.label">
            <div class="fact-row">
              <dt>{{ fact.label }}</dt>
              <dd>{{ fact.value }}</dd>
            </div>
          </template>
        </dl>
      </section>

      <section class="profile-section" v-if="profile.contentHtml">
        <h2>纪念正文</h2>
        <div class="story" v-html="profile.contentHtml"></div>
      </section>

      <section class="profile-section" v-if="profile.websites.length">
        <h2>外部链接</h2>
        <div class="external-links">
          <a v-for="site in profile.websites" :key="site.url" :href="site.url" target="_blank" rel="noopener">
            {{ site.label }}
          </a>
        </div>
      </section>
    </article>
    <div v-else-if="loading" class="placeholder">
      <h1>加载中…</h1>
    </div>
    <div v-else class="placeholder">
      <h1>无法加载预览</h1>
      <p>请确认你已登录管理员账号，或回到队列重新进入。</p>
    </div>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { adminAPI } from '@/api/client';

const props = defineProps<{ draftId: string }>();
const profile = ref<any | null>(null);
const loading = ref(true);

onMounted(async () => {
  try {
    const r = await adminAPI.draftPreview(props.draftId);
    profile.value = r.profile;
  } catch {
    profile.value = null;
  } finally {
    loading.value = false;
  }
});

function firstGlyph(name: string): string {
  return Array.from(String(name || '勿').trim())[0] || '勿';
}

function formatDate(value: string): string {
  if (!value) return '日期待考';
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  const [y, m, d] = value.split('-');
  return `${y}.${m}.${d}`;
}
</script>

<style scoped>
.preview-wrap {
  width: min(960px, calc(100% - 2rem));
  margin: 0 auto;
}
.placeholder {
  padding: 3rem 1rem;
  text-align: center;
  color: var(--muted);
}
</style>
