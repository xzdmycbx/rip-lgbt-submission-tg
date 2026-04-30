<template>
  <SiteHeader />
  <main>
    <article class="profile" v-if="profile">
      <RouterLink class="back-link" to="/">返回索引</RouterLink>
      <header class="profile-hero">
        <div class="profile-photo">
          <img v-if="profile.profileUrl" class="profile-avatar" :src="profile.profileUrl" alt="" />
          <div v-else class="profile-mark" aria-hidden="true">{{ firstGlyph(profile.name) }}</div>
        </div>
        <div>
          <p class="eyebrow">MEMORIAL ENTRY</p>
          <h1>{{ profile.name }}</h1>
          <p class="profile-date">{{ formatDate(profile.departure) }}</p>
          <p class="profile-desc">{{ profile.desc || '此处保留这位逝者的基础信息。' }}</p>
        </div>
      </header>

      <section class="profile-section" v-if="profile.facts.length">
        <h2>公开信息</h2>
        <dl class="facts">
          <template v-for="fact in profile.facts" :key="fact.label">
            <dt>{{ fact.label }}</dt>
            <dd>{{ fact.value }}</dd>
          </template>
        </dl>
      </section>

      <section class="profile-section" v-if="profile.contentHtml">
        <h2>纪念正文</h2>
        <div class="story" v-html="profile.contentHtml"></div>
      </section>

      <EngagementWidget :id="profile.id" :name="profile.name" />

      <section class="profile-section" v-if="profile.websites.length">
        <h2>外部链接</h2>
        <p>
          <a v-for="site in profile.websites" :key="site.url" :href="site.url" target="_blank" rel="noopener" style="margin-right:1rem;">
            {{ site.label }}
          </a>
        </p>
      </section>
    </article>
    <p v-else-if="loading" class="placeholder"><h1>加载中…</h1></p>
    <article v-else class="placeholder">
      <h1>未找到这位逝者</h1>
      <RouterLink class="button" to="/">返回索引</RouterLink>
    </article>
  </main>
  <SiteFooter />
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import { memorialsAPI, type Profile } from '@/api/client';
import SiteHeader from '@/components/SiteHeader.vue';
import SiteFooter from '@/components/SiteFooter.vue';
import EngagementWidget from '@/components/EngagementWidget.vue';

const props = defineProps<{ id: string }>();
const profile = ref<Profile | null>(null);
const loading = ref(true);

async function load() {
  loading.value = true;
  try {
    profile.value = await memorialsAPI.get(props.id);
  } catch {
    profile.value = null;
  } finally {
    loading.value = false;
  }
}

onMounted(load);
watch(() => props.id, load);

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
