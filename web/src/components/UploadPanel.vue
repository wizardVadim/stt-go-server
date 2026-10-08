<script setup lang="ts">
import { onUnmounted, ref, watch } from "vue";
import AppIcon from "./AppIcon.vue";
import {
  acceptMedia,
  checkFile,
  MAX_DURATION,
  readDuration,
} from "../utils/media";
import { duration, fileSize } from "../utils/format";

const props = defineProps<{
  uploading: boolean;
  progress: number;
  error: string;
  success: number;
  demo: boolean;
}>();
const emit = defineEmits<{ upload: [file: File]; cancel: [] }>();
const input = ref<HTMLInputElement>();
const file = ref<File | null>(null);
const seconds = ref<number | null>(null);
const checking = ref(false);
const validationError = ref("");
const dragging = ref(false);
let version = 0;

async function choose(files: FileList | null) {
  if (props.uploading || !files?.length) return;
  const token = ++version;
  file.value = null;
  seconds.value = null;
  validationError.value = "";
  checking.value = false;
  if (files.length > 1) {
    validationError.value =
      "Добавляйте по одному файлу. Остальные можно отправить следом.";
    return;
  }
  const next = files[0]!;
  const error = checkFile(next);
  if (error) {
    validationError.value = error;
    return;
  }
  file.value = next;
  checking.value = true;
  const value = await readDuration(next);
  if (token !== version) return;
  checking.value = false;
  seconds.value = value;
  if (value !== null && value > MAX_DURATION)
    validationError.value = `Запись длится ${duration(value)}. Максимум — 10 минут. Выберите более короткий файл.`;
}

function reset() {
  version++;
  file.value = null;
  seconds.value = null;
  checking.value = false;
  validationError.value = "";
  if (input.value) input.value.value = "";
}
function drop(event: DragEvent) {
  dragging.value = false;
  void choose(event.dataTransfer?.files || null);
}
watch(() => props.success, reset);
onUnmounted(() => {
  version++;
});
</script>

<template>
  <section id="upload" class="upload-card" aria-labelledby="upload-title">
    <div class="section-heading">
      <div class="heading-with-icon">
        <span class="small-icon"><AppIcon name="plus" :size="18" /></span>
        <h2 id="upload-title">Новая расшифровка</h2>
      </div>
      <span class="limit-label"
        ><AppIcon name="clock" :size="14" /> до 10 минут</span
      >
    </div>
    <div
      class="dropzone"
      :class="{
        'is-dragging': dragging,
        'has-file': file,
        'has-error': validationError,
      }"
      @dragover.prevent="dragging = !uploading"
      @dragleave.prevent="dragging = false"
      @drop.prevent="drop"
    >
      <input
        ref="input"
        class="visually-hidden"
        type="file"
        :accept="acceptMedia"
        :disabled="uploading"
        aria-label="Выбрать аудио или видео"
        @change="choose(($event.target as HTMLInputElement).files)"
      />
      <div class="upload-symbol">
        <AppIcon :name="file ? 'file' : 'upload'" :size="27" />
      </div>
      <template v-if="file">
        <strong class="upload-filename">{{ file.name }}</strong>
        <p>
          {{ fileSize(file.size) }} <span class="dot-separator">·</span>
          {{
            checking
              ? "Проверяем длительность…"
              : seconds !== null
                ? duration(seconds)
                : "Длительность проверит сервер"
          }}
        </p>
        <button v-if="!uploading" class="text-button" @click="reset">
          Выбрать другой файл
        </button>
      </template>
      <template v-else>
        <strong>Перетащите сюда аудио или видео</strong>
        <p>
          или
          <button
            class="inline-link"
            :disabled="uploading"
            @click="input?.click()"
          >
            выберите файл
          </button>
          на устройстве
        </p>
        <span class="formats">MP3, WAV, M4A, MP4, WebM и другие</span>
      </template>
    </div>
    <p v-if="validationError || error" class="inline-error" role="alert">
      <AppIcon name="alert" :size="17" />{{ validationError || error }}
    </p>
    <p v-if="demo" class="demo-upload-note">
      Демо: файл не отправляется и не распознаётся. Результат — заранее
      подготовленный пример.
    </p>
    <div v-if="uploading" class="upload-progress" role="status">
      <div>
        <span>{{
          progress < 100
            ? `Загрузка файла · ${progress}%`
            : "Файл передан · сервер проверяет запись…"
        }}</span
        ><button class="text-button" @click="emit('cancel')">
          Остановить передачу
        </button>
      </div>
      <progress :value="progress" max="100" aria-label="Загрузка файла" />
    </div>
    <div v-else class="upload-bottom">
      <span class="language-note"
        ><span class="language-mark">EN</span>Только английская речь</span
      >
      <button
        class="button primary"
        :disabled="!file || checking || !!validationError"
        @click="file && emit('upload', file)"
      >
        {{ checking ? "Проверяем файл…" : "Распознать"
        }}<AppIcon name="arrow" :size="17" />
      </button>
    </div>
  </section>
</template>
