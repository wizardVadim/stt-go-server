<script setup lang="ts">
import { computed, ref } from "vue";
import AppIcon from "./AppIcon.vue";
import StatusBadge from "./StatusBadge.vue";
import { isActive, type Job, type JobFilter } from "../types/job";
import { dateLabel, duration } from "../utils/format";

const props = defineProps<{
  jobs: Job[];
  selectedId: string;
  filter: JobFilter;
  loading: boolean;
}>();
const emit = defineEmits<{
  select: [id: string];
  filter: [value: JobFilter];
  refresh: [];
}>();
const query = ref("");
const filtered = computed(() =>
  props.jobs.filter(
    (job) =>
      (props.filter === "all" ||
        (props.filter === "active"
          ? isActive(job)
          : job.status === "completed")) &&
      job.filename
        .toLocaleLowerCase()
        .includes(query.value.toLocaleLowerCase().trim()),
  ),
);
const filters: { key: JobFilter; label: string }[] = [
  { key: "all", label: "Все" },
  { key: "active", label: "В работе" },
  { key: "completed", label: "Готовые" },
];
</script>

<template>
  <section class="jobs-panel" aria-labelledby="jobs-title">
    <div class="section-heading jobs-heading">
      <h2 id="jobs-title">
        Мои записи <span class="count">{{ jobs.length }}</span>
      </h2>
      <button
        class="icon-button"
        aria-label="Обновить записи"
        @click="emit('refresh')"
      >
        <AppIcon name="refresh" :size="17" />
      </button>
    </div>
    <label class="search-field"
      ><AppIcon name="search" :size="17" /><input
        v-model="query"
        type="search"
        placeholder="Найти запись"
        aria-label="Поиск записей"
    /></label>
    <div class="filter-tabs" aria-label="Фильтр записей">
      <button
        v-for="item in filters"
        :key="item.key"
        :class="{ selected: filter === item.key }"
        :aria-pressed="filter === item.key"
        @click="emit('filter', item.key)"
      >
        {{ item.label }}
      </button>
    </div>
    <div v-if="loading" class="list-empty" role="status">
      <span class="spinner" />Загружаем записи…
    </div>
    <div v-else-if="!filtered.length" class="list-empty">
      <AppIcon :name="query ? 'search' : 'headphones'" :size="28" /><strong>{{
        query
          ? "Ничего не нашлось"
          : jobs.length
            ? "Здесь пока пусто"
            : "Первая запись — впереди"
      }}</strong>
      <p>
        {{
          query
            ? "Попробуйте другое название."
            : jobs.length
              ? "Попробуйте другой фильтр."
              : "Добавьте файл, и он появится в этом списке."
        }}
      </p>
    </div>
    <ul v-else class="job-list">
      <li v-for="job in filtered" :key="job.id">
        <button
          class="job-item"
          :class="{ selected: selectedId === job.id }"
          :aria-pressed="selectedId === job.id"
          @click="emit('select', job.id)"
        >
          <span
            class="file-icon"
            :class="{ complete: job.status === 'completed' }"
            ><AppIcon
              :name="
                job.status === 'completed'
                  ? 'file'
                  : isActive(job)
                    ? 'wave'
                    : 'headphones'
              "
              :size="21"
          /></span>
          <span class="job-summary"
            ><strong :title="job.filename">{{ job.filename }}</strong
            ><span class="job-metadata"
              >{{ duration(job.duration_seconds) }} <span>·</span> EN</span
            ><StatusBadge :status="job.status" /><span
              v-if="job.status === 'transcribing' && job.progress !== null"
              class="job-mini-progress"
              ><span
                :style="{
                  width: `${Math.min(99, Math.round(job.progress * 100))}%`,
                }" /></span
          ></span>
          <AppIcon name="chevron" :size="15" class="job-chevron" />
          <span class="visually-hidden">{{ dateLabel(job.created_at) }}</span>
        </button>
      </li>
    </ul>
    <div class="queue-note">
      <AppIcon name="clock" :size="15" /><span
        >Записи обрабатываются по очереди.</span
      >
    </div>
  </section>
</template>
