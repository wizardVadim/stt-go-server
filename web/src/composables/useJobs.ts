import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { api, isDemo } from "../api";
import { isActive, type Job, type Transcript } from "../types/job";

export function useJobs() {
  const jobs = ref<Job[]>([]);
  const selectedId = ref("");
  const connected = ref(false);
  const loading = ref(true);
  const listError = ref("");
  const actionError = ref("");
  const busy = ref(false);
  const transcript = ref<Transcript | null>(null);
  const transcriptLoading = ref(false);
  const transcriptError = ref("");
  const uploading = ref(false);
  const uploadProgress = ref(0);
  const uploadError = ref("");
  const uploadSuccess = ref(0);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let listController: AbortController | undefined;
  let resultController: AbortController | undefined;
  let uploadController: AbortController | undefined;
  let stopped = false;
  const storageKey = `slovo-selection-${isDemo ? "demo" : "live"}`;
  try {
    selectedId.value = localStorage.getItem(storageKey) || "";
  } catch {
    /* optional preference */
  }
  const selected = computed(
    () => jobs.value.find((job) => job.id === selectedId.value) || null,
  );
  const activeCount = computed(() => jobs.value.filter(isActive).length);
  const completedCount = computed(
    () => jobs.value.filter((j) => j.status === "completed").length,
  );
  const errorMessage = (error: unknown) =>
    error instanceof Error ? error.message : "Не удалось выполнить действие.";

  function select(id: string) {
    selectedId.value = id;
    actionError.value = "";
    try {
      localStorage.setItem(storageKey, id);
    } catch {
      /* optional preference */
    }
  }

  async function refresh() {
    if (stopped) return;
    listController?.abort();
    const controller = new AbortController();
    listController = controller;
    try {
      const result = await api.list(controller.signal);
      if (controller.signal.aborted || stopped) return;
      jobs.value = result.sort(
        (a, b) => Date.parse(b.created_at) - Date.parse(a.created_at),
      );
      connected.value = true;
      listError.value = "";
      if (!jobs.value.some((j) => j.id === selectedId.value))
        select(jobs.value[0]?.id || "");
    } catch (error) {
      if (controller.signal.aborted || stopped) return;
      connected.value = false;
      listError.value = errorMessage(error);
    } finally {
      if (!controller.signal.aborted && !stopped) loading.value = false;
    }
  }

  async function poll() {
    await refresh();
    if (!stopped) timer = setTimeout(poll, connected.value ? 3000 : 7000);
  }

  async function loadTranscript() {
    resultController?.abort();
    transcript.value = null;
    transcriptError.value = "";
    transcriptLoading.value = false;
    const job = selected.value;
    if (job?.status !== "completed") return;
    const controller = new AbortController();
    resultController = controller;
    transcriptLoading.value = true;
    try {
      const result = await api.transcript(job.id, controller.signal);
      if (!controller.signal.aborted && selectedId.value === job.id)
        transcript.value = result;
    } catch (error) {
      if (!controller.signal.aborted)
        transcriptError.value = errorMessage(error);
    } finally {
      if (!controller.signal.aborted) transcriptLoading.value = false;
    }
  }
  watch(
    () => `${selected.value?.id}:${selected.value?.status}`,
    loadTranscript,
  );

  async function upload(file: File) {
    if (uploading.value) return;
    uploading.value = true;
    uploadProgress.value = 0;
    uploadError.value = "";
    const controller = new AbortController();
    uploadController = controller;
    try {
      const job = await api.create(
        file,
        (value) => {
          uploadProgress.value = value;
        },
        controller.signal,
      );
      if (stopped) return;
      jobs.value = [job, ...jobs.value.filter((j) => j.id !== job.id)];
      select(job.id);
      uploadSuccess.value++;
      await refresh();
    } catch (error) {
      if (controller.signal.aborted)
        uploadError.value =
          "Передача остановлена. Если сервер уже принял файл, запись появится в списке — её можно отменить отдельно.";
      else uploadError.value = errorMessage(error);
      await refresh();
    } finally {
      uploading.value = false;
    }
  }

  async function action(kind: "cancel" | "retry" | "remove", job: Job) {
    if (busy.value) return;
    busy.value = true;
    actionError.value = "";
    try {
      if (kind === "remove") {
        await api.remove(job.id);
        jobs.value = jobs.value.filter((j) => j.id !== job.id);
      } else {
        const result = await api[kind](job.id);
        jobs.value = jobs.value.map((j) => (j.id === result.id ? result : j));
      }
      await refresh();
    } catch (error) {
      actionError.value = errorMessage(error);
    } finally {
      busy.value = false;
    }
  }

  onMounted(poll);
  onUnmounted(() => {
    stopped = true;
    clearTimeout(timer);
    listController?.abort();
    resultController?.abort();
    uploadController?.abort();
  });
  return {
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
    cancelUpload: () => uploadController?.abort(),
  };
}
