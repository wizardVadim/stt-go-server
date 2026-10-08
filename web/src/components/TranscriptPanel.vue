<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import AppIcon from "./AppIcon.vue";
import StatusBadge from "./StatusBadge.vue";
import { isActive, type Job, type Transcript } from "../types/job";
import { dateLabel, duration, wordCount } from "../utils/format";
import { downloadTranscript } from "../utils/export";

const props = defineProps<{
  job: Job | null;
  transcript: Transcript | null;
  loading: boolean;
  error: string;
  actionError: string;
  busy: boolean;
  demo: boolean;
}>();
const emit = defineEmits<{
  cancel: [job: Job];
  retry: [job: Job];
  remove: [job: Job];
  reload: [];
}>();
const tab = ref<"text" | "segments">("text");
const format = ref<"txt" | "srt" | "json">("txt");
const copied = ref(false);
const copyError = ref("");
let copyTimer: ReturnType<typeof setTimeout> | undefined;
const percent = computed(() =>
  props.job?.progress === null || props.job?.progress === undefined
    ? null
    : Math.min(99, Math.round(props.job.progress * 100)),
);
watch(
  () => props.job?.id,
  () => {
    tab.value = "text";
    copied.value = false;
    copyError.value = "";
    clearTimeout(copyTimer);
  },
);

async function copy() {
  if (!props.transcript) return;
  copyError.value = "";
  try {
    if (window.isSecureContext && navigator.clipboard)
      await navigator.clipboard.writeText(props.transcript.text);
    else {
      const area = document.createElement("textarea");
      area.value = props.transcript.text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      const focused = document.activeElement as HTMLElement | null;
      document.body.append(area);
      area.select();
      const success = document.execCommand("copy");
      area.remove();
      focused?.focus();
      if (!success) throw new Error("copy");
    }
    copied.value = true;
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => {
      copied.value = false;
    }, 2500);
  } catch {
    copyError.value =
      "Браузер не разрешил копирование. Выделите текст вручную или скачайте TXT.";
  }
}
onUnmounted(() => clearTimeout(copyTimer));
</script>

<template>
  <section
    class="transcript-panel"
    aria-labelledby="transcript-title"
    :aria-busy="loading"
  >
    <template v-if="!job">
      <div class="result-placeholder">
        <div class="paper-illustration" aria-hidden="true">
          <span class="paper-tag">Aa</span><i /><i /><i /><i /><span
            class="paper-check"
            ><AppIcon name="check" :size="19"
          /></span>
        </div>
        <span class="eyebrow">МЕСТО ДЛЯ ВАШИХ МЫСЛЕЙ</span>
        <h2 id="transcript-title">Здесь появится текст</h2>
        <p>
          Загрузите запись или выберите её в списке.<br />Вся лекция — в удобном
          для чтения виде.
        </p>
      </div>
    </template>
    <template v-else>
      <header class="transcript-heading">
        <div class="result-eyebrow">
          <span class="eyebrow">РАСШИФРОВКА</span
          ><StatusBadge :status="job.status" />
        </div>
        <h2 id="transcript-title">{{ job.filename }}</h2>
        <div class="transcript-meta">
          <span
            ><AppIcon name="clock" :size="14" />{{
              duration(job.duration_seconds)
            }}</span
          ><span>English</span><span>{{ dateLabel(job.created_at) }}</span>
        </div>
      </header>
      <p v-if="demo" class="transcript-demo">
        Демонстрационный пример. Текст и время не относятся к загруженному
        файлу.
      </p>
      <div
        v-if="job.status === 'completed' && transcript"
        class="transcript-toolbar"
      >
        <div class="view-tabs" aria-label="Вид расшифровки">
          <button
            :class="{ selected: tab === 'text' }"
            :aria-pressed="tab === 'text'"
            @click="tab = 'text'"
          >
            Текст</button
          ><button
            :class="{ selected: tab === 'segments' }"
            :aria-pressed="tab === 'segments'"
            @click="tab = 'segments'"
          >
            Таймкоды
          </button>
        </div>
        <button class="copy-button" @click="copy">
          <AppIcon :name="copied ? 'check' : 'copy'" :size="15" />{{
            copied ? "Скопировано" : "Копировать"
          }}
        </button>
      </div>
      <p v-if="copyError" class="inline-error padded" role="alert">
        {{ copyError }}
      </p>
      <div v-if="loading" class="result-state" role="status">
        <span class="spinner" />
        <h3>Загружаем полный текст</h3>
      </div>
      <div v-else-if="error" class="result-state">
        <AppIcon name="alert" :size="32" />
        <h3>Не удалось загрузить текст</h3>
        <p role="alert">{{ error }}</p>
        <button class="button secondary" @click="emit('reload')">
          Попробовать ещё раз
        </button>
      </div>
      <template v-else-if="job.status === 'completed' && transcript">
        <div v-if="!transcript.text.trim()" class="result-state">
          <AppIcon name="headphones" :size="30" />
          <h3>Речь не обнаружена</h3>
          <p>Проверьте, что в записи слышна английская речь.</p>
        </div>
        <div v-else-if="tab === 'text'" class="transcript-text" lang="en">
          {{ transcript.text }}
        </div>
        <div v-else class="transcript-segments">
          <p v-if="!transcript.segments.length" class="muted">
            Для этой записи нет временных меток.
          </p>
          <div
            v-for="(segment, index) in transcript.segments"
            :key="index"
            class="segment"
          >
            <span class="segment-time">{{ duration(segment.start) }}</span>
            <p lang="en">{{ segment.text }}</p>
          </div>
        </div>
        <footer class="transcript-footer">
          <span class="word-count"
            >{{ wordCount(transcript.text).toLocaleString("ru-RU") }} слов
            <span>·</span> полный текст</span
          >
          <div class="download-controls">
            <select v-model="format" aria-label="Формат скачивания">
              <option value="txt">TXT</option>
              <option value="srt" :disabled="!transcript.segments.length">
                SRT
              </option>
              <option value="json">JSON</option></select
            ><button
              class="button secondary small"
              @click="downloadTranscript(transcript, job.filename, format)"
            >
              <AppIcon name="download" :size="16" />Скачать
            </button>
          </div>
        </footer>
      </template>
      <div
        v-else-if="isActive(job)"
        class="result-state processing-state"
        role="status"
      >
        <div class="processing-wave" aria-hidden="true">
          <i /><i /><i /><i /><i />
        </div>
        <h3>
          {{
            job.status === "queued"
              ? "Ваша запись в очереди"
              : job.status === "preparing"
                ? "Готовим аудио к распознаванию"
                : "Превращаем речь в текст"
          }}
        </h3>
        <p>
          {{
            job.status === "queued"
              ? "Обработка начнётся, когда освободится очередь."
              : job.status === "preparing"
                ? "Проверяем запись и извлекаем звуковую дорожку."
                : "Это может занять больше времени, чем длится сама запись."
          }}
        </p>
        <div
          v-if="job.status === 'transcribing' && percent !== null"
          class="recognition-progress"
        >
          <progress
            :value="percent"
            max="100"
            aria-label="Прогресс распознавания"
          /><span>{{ percent }}%</span>
        </div>
        <span class="background-note"
          >Можно закрыть страницу и вернуться позже.</span
        >
        <button
          class="text-button danger-text"
          :disabled="busy"
          @click="emit('cancel', job)"
        >
          Отменить обработку
        </button>
      </div>
      <div
        v-else-if="job.status === 'failed' || job.status === 'cancelled'"
        class="result-state"
      >
        <AppIcon
          :name="job.status === 'failed' ? 'alert' : 'close'"
          :size="32"
        />
        <h3>
          {{
            job.status === "failed"
              ? "Не удалось распознать запись"
              : "Обработка отменена"
          }}
        </h3>
        <p>
          {{
            job.error ||
            (job.status === "failed"
              ? "Попробуйте запустить обработку ещё раз."
              : "Вы можете запустить эту запись заново.")
          }}
        </p>
        <button
          class="button primary"
          :disabled="busy"
          @click="emit('retry', job)"
        >
          <AppIcon name="refresh" :size="16" />Повторить
        </button>
      </div>
      <p v-if="actionError" class="inline-error padded" role="alert">
        {{ actionError }}
      </p>
      <div v-if="!isActive(job)" class="record-actions">
        <span>Текст на языке оригинала</span
        ><button
          class="text-button"
          :disabled="busy"
          @click="emit('remove', job)"
        >
          <AppIcon name="trash" :size="14" />Удалить запись
        </button>
      </div>
    </template>
  </section>
</template>
