<template>
  <article class="person-card">
    <RouterLink class="person-link" :to="`/memorial/${person.id}`">
      <img v-if="person.profileUrl" :src="person.profileUrl" alt="" class="person-avatar" loading="lazy" decoding="async" />
      <div v-else class="petal" aria-hidden="true">{{ firstGlyph(person.name) }}</div>
      <div class="person-main">
        <strong class="person-name">{{ person.name }}</strong>
        <span class="person-date">{{ formatDate(person.departure) }}</span>
        <span class="person-desc">{{ person.desc || '此处保留这位逝者的基础信息。' }}</span>
      </div>
      <span class="person-id" v-if="person.id">{{ person.id }}</span>
    </RouterLink>
  </article>
</template>

<script setup lang="ts">
import type { Person } from '@/api/client';

defineProps<{ person: Person }>();

function firstGlyph(name: string): string {
  const chars = Array.from(String(name || '勿').trim());
  return chars[0] || '勿';
}

function formatDate(value: string): string {
  if (!value) return '日期待考';
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  const [y, m, d] = value.split('-');
  return `${y}.${m}.${d}`;
}
</script>
