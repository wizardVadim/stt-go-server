<script setup lang="ts">
import { nextTick, ref } from "vue";
import { isDemo } from "../api";
import AppIcon from "../components/AppIcon.vue";
import UploadPanel from "../components/UploadPanel.vue";
import JobList from "../components/JobList.vue";
import TranscriptPanel from "../components/TranscriptPanel.vue";
import { useJobs } from "../composables/useJobs";
import type { Job, JobFilter } from "../types/job";

const {
  jobs,
  selected,
  selectedId,
  connected,
  loading,
  listError,
  actionError,
  busy,
  transcript,
  transcriptLoading,
  transcriptError,
  uploading,
  uploadProgress,
  uploadError,
  uploadSuccess,
  activeCount,
  completedCount,
  select,
  refresh,
  loadTranscript,
  upload,
  action,
  cancelUpload,
} = useJobs();
const filter = ref<JobFilter>("all");
const deleteDialog = ref<HTMLDialogElement>();
const deleting = ref<Job | null>(null);

function focusUpload() {
  document
    .getElementById("upload")
    ?.scrollIntoView({ behavior: "smooth", block: "center" });
  document
    .querySelector<HTMLInputElement>("#upload input[type=file]")
    ?.focus({ preventScroll: true });
}
async function askDelete(job: Job) {
  deleting.value = job;
  await nextTick();
  deleteDialog.value?.showModal();
}
async function confirmDelete() {
  const job = deleting.value;
  deleteDialog.value?.close();
  if (job) await action("remove", job);
  deleting.value = null;
}
</script>

<template>
  <a class="skip-link" href="#main">К содержимому</a>
  <div class="app-shell">
    <aside class="sidebar" aria-label="Навигация">
      <a class="brand" href="#main"
        ><span class="brand-symbol"><AppIcon name="wave" :size="25" /></span
        ><span>слово<span class="brand-period">.</span></span></a
      >
      <div class="sidebar-caption">ВАШЕ ПРОСТРАНСТВО</div>
      <nav class="sidebar-nav" aria-label="Записи">
        <button :class="{ active: filter === 'all' }" @click="filter = 'all'">
          <AppIcon name="grid" />Все записи<span>{{ jobs.length }}</span>
        </button>
        <button
          :class="{ active: filter === 'active' }"
          @click="filter = 'active'"
        >
          <AppIcon name="wave" />В работе<span>{{ activeCount }}</span>
        </button>
        <button
          :class="{ active: filter === 'completed' }"
          @click="filter = 'completed'"
        >
          <AppIcon name="check" />Готовые<span>{{ completedCount }}</span>
        </button>
      </nav>
      <button class="sidebar-upload" @click="focusUpload">
        <AppIcon name="plus" :size="17" />Добавить запись
      </button>
      <div class="sidebar-bottom">
        <div class="sidebar-note">
          <AppIcon name="headphones" :size="24" />
          <p>
            Слушайте внимательно.<br /><span>К тексту можно вернуться.</span>
          </p>
        </div>
        <div class="server-state">
          <span class="connection-dot" :class="{ online: connected }" />
          <div>
            {{
              isDemo
                ? "Демонстрация"
                : connected
                  ? "Сервер подключён"
                  : loading
                    ? "Подключение…"
                    : "Сервер недоступен"
            }}<small>{{
              isDemo ? "Пример работы интерфейса" : "Локальное распознавание"
            }}</small>
          </div>
        </div>
      </div>
    </aside>
    <main id="main" class="main-content">
      <header class="topbar">
        <span>Аудио <AppIcon name="arrow" :size="14" /> текст</span
        ><span class="private-label"
          ><AppIcon name="shield" :size="15" />Ваши записи остаются у вас</span
        >
      </header>
      <div class="workspace">
        <div v-if="isDemo" class="notice demo-notice">
          <span class="demo-pill">ДЕМО</span
          ><span
            >Посмотрите, как всё устроено. Здесь примеры записей, а не настоящее
            распознавание.</span
          >
        </div>
        <div
          v-else-if="listError"
          class="notice connection-notice"
          role="status"
        >
          <AppIcon name="alert" :size="19" /><span>{{ listError }}</span
          ><button class="text-button" @click="refresh">Подключиться</button>
        </div>
        <section class="intro">
          <div>
            <span class="eyebrow">МЕНЬШЕ ПЕРЕПИСЫВАТЬ. БОЛЬШЕ ПОНИМАТЬ.</span>
            <h1>Из речи — <span>в текст.</span></h1>
            <p>Английские лекции и заметки, к которым легко вернуться.</p>
          </div>
          <span class="intro-label"
            ><span class="small-wave"><AppIcon name="wave" :size="25" /></span
            >Для внимательного чтения</span
          >
        </section>
        <UploadPanel
          :uploading="uploading"
          :progress="uploadProgress"
          :error="uploadError"
          :success="uploadSuccess"
          :demo="isDemo"
          @upload="upload"
          @cancel="cancelUpload"
        />
        <div class="workspace-label">
          <span>ВАША БИБЛИОТЕКА</span
          ><span>Речь проходит. Текст остаётся.</span>
        </div>
        <div class="library-grid">
          <JobList
            :jobs="jobs"
            :selected-id="selectedId"
            :filter="filter"
            :loading="loading"
            @select="select"
            @filter="filter = $event"
            @refresh="refresh"
          />
          <TranscriptPanel
            :job="selected"
            :transcript="transcript"
            :loading="transcriptLoading"
            :error="transcriptError"
            :action-error="actionError"
            :busy="busy"
            :demo="isDemo"
            @cancel="action('cancel', $event)"
            @retry="action('retry', $event)"
            @remove="askDelete"
            @reload="loadTranscript"
          />
        </div>
        <footer class="page-footer">
          <span>слово. <span>Слышать. Понимать. Сохранять.</span></span
          ><span
            >English only <span class="dot-separator">·</span> до 10 минут</span
          >
        </footer>
      </div>
    </main>
  </div>
  <dialog
    ref="deleteDialog"
    class="confirm-dialog"
    aria-labelledby="delete-title"
    @close="deleting = null"
  >
    <div class="dialog-icon"><AppIcon name="trash" :size="23" /></div>
    <h2 id="delete-title">Удалить запись?</h2>
    <p>
      Файл «{{ deleting?.filename }}» и его расшифровка будут удалены. Вернуть
      их не получится.
    </p>
    <div class="dialog-actions">
      <button class="button secondary" autofocus @click="deleteDialog?.close()">
        Оставить</button
      ><button class="button destructive" @click="confirmDelete">
        Удалить
      </button>
    </div>
  </dialog>
</template>
