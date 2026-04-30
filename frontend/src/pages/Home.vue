<template>
  <SiteHeader />
  <main>
    <section class="hero">
      <div class="hero-copy">
        <p class="eyebrow">REST IN PEACE · IN MEMORY</p>
        <h1>勿忘我</h1>
        <p class="lede">
          名字不是装饰。名字是一个人来过、被爱过、仍应被温柔提起的证据。本站正在转为独立维护，只展示经授权或自建整理的纪念条目。
        </p>
        <div class="hero-actions">
          <a v-if="people.length" class="button primary" href="#memorials">查看 {{ people.length }} 位</a>
          <RouterLink v-else class="button primary" to="/submit">提交纪念条目</RouterLink>
        </div>
      </div>
      <aside class="hero-panel" aria-label="索引统计">
        <div>
          <span class="stat-number">{{ people.length }}</span>
          <span class="stat-label">自建条目</span>
        </div>
        <div>
          <span class="stat-number">{{ rangeLabel }}</span>
          <span class="stat-label">时间范围</span>
        </div>
        <div>
          <span class="stat-number">投稿</span>
          <span class="stat-label">通过 Telegram 接受独立提交</span>
        </div>
      </aside>
    </section>

    <section class="care-note">
      <p>
        <strong>阅读提醒</strong>：原始条目可能包含自杀自伤、家庭暴力、性暴力、物质滥用等创伤内容。请在自己状态稳定时阅读；如果感到不适，请先离开页面并寻求可信赖的人或专业支持。
      </p>
    </section>

    <section id="memorials">
      <div class="section-head">
        <div>
          <p class="eyebrow">INDEX</p>
          <h2>纪念索引</h2>
        </div>
        <label class="search">
          <input v-model="query" type="search" placeholder="搜索姓名、ID 或日期" autocomplete="off" />
        </label>
      </div>
      <p class="result-count">
        {{ filtered.length ? `显示 ${filtered.length} 位` : '暂无公开条目' }}
      </p>
      <div class="people-grid">
        <MemorialCard v-for="person in filtered" :key="person.id" :person="person" />
      </div>
    </section>
  </main>
  <SiteFooter />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { memorialsAPI, type Person } from '@/api/client';
import SiteHeader from '@/components/SiteHeader.vue';
import SiteFooter from '@/components/SiteFooter.vue';
import MemorialCard from '@/components/MemorialCard.vue';

const people = ref<Person[]>([]);
const query = ref('');

onMounted(async () => {
  try {
    const r = await memorialsAPI.list();
    people.value = r.people || [];
  } catch {
    // ignore: empty list rendered
  }
});

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase();
  if (!q) return people.value;
  return people.value.filter(
    (p) => `${p.name} ${p.id} ${p.departure} ${p.desc}`.toLowerCase().includes(q)
  );
});

const rangeLabel = computed(() => {
  const dated = people.value
    .map((p) => p.departure)
    .filter((d) => /^\d{4}-\d{2}-\d{2}$/.test(d))
    .sort();
  if (dated.length < 1) return '待收录';
  const start = dated[0]!.slice(0, 4);
  const end = dated[dated.length - 1]!.slice(0, 4);
  return start === end ? start : `${start}-${end}`;
});
</script>
