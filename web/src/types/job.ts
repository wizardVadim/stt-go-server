export const statuses = [
  "queued",
  "preparing",
  "transcribing",
  "completed",
  "failed",
  "cancelled",
] as const;
export type JobStatus = (typeof statuses)[number];
export type JobFilter = "all" | "active" | "completed";

export interface Job {
  id: string;
  filename: string;
  size_bytes: number;
  duration_seconds: number | null;
  language: "en";
  status: JobStatus;
  /** Real processed fraction, 0..1. Null means unknown. */
  progress: number | null;
  created_at: string;
  updated_at: string;
  error: string | null;
}

export interface Segment {
  start: number;
  end: number;
  text: string;
}
export interface Transcript {
  language: "en";
  text: string;
  segments: Segment[];
}

export interface JobsApi {
  list(signal?: AbortSignal): Promise<Job[]>;
  create(
    file: File,
    progress: (percent: number) => void,
    signal: AbortSignal,
  ): Promise<Job>;
  transcript(id: string, signal?: AbortSignal): Promise<Transcript>;
  cancel(id: string): Promise<Job>;
  retry(id: string): Promise<Job>;
  remove(id: string): Promise<void>;
}

export const statusLabels: Record<JobStatus, string> = {
  queued: "В очереди",
  preparing: "Подготовка",
  transcribing: "Распознаётся",
  completed: "Готово",
  failed: "Ошибка",
  cancelled: "Отменено",
};
export const isActive = (job: Job) =>
  ["queued", "preparing", "transcribing"].includes(job.status);
