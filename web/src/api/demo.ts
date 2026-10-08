import {
  isActive,
  type Job,
  type JobsApi,
  type Transcript,
} from "../types/job";

const key = "slovo-demo-v1";
const example: Transcript = {
  language: "en",
  text: "Learning is not simply a matter of collecting information. It is a process of connecting new ideas to what we already know.\n\nImagine that you are learning a new language. Reading a definition is a useful first step, but using the word in a conversation makes the idea your own. The same principle applies to almost any subject.\n\nOne practical approach is to pause after a short lesson and explain its main ideas in your own words. If an explanation feels difficult, that is a useful signal: you have found something worth revisiting.\n\nGive yourself time to reflect. A thoughtful question can be more valuable than a page of notes. Understanding grows when we return to an idea, test it, and see how it fits into a larger picture.",
  segments: [
    {
      start: 0,
      end: 18.2,
      text: "Learning is not simply a matter of collecting information. It is a process of connecting new ideas to what we already know.",
    },
    {
      start: 18.2,
      end: 43.5,
      text: "Imagine that you are learning a new language. Reading a definition is a useful first step, but using the word in a conversation makes the idea your own. The same principle applies to almost any subject.",
    },
    {
      start: 43.5,
      end: 69.1,
      text: "One practical approach is to pause after a short lesson and explain its main ideas in your own words. If an explanation feels difficult, that is a useful signal: you have found something worth revisiting.",
    },
    {
      start: 69.1,
      end: 92,
      text: "Give yourself time to reflect. A thoughtful question can be more valuable than a page of notes. Understanding grows when we return to an idea, test it, and see how it fits into a larger picture.",
    },
  ],
};

function initial(): Job[] {
  const now = new Date().toISOString();
  return [
    {
      id: "demo-learning",
      filename: "The art of learning.mp4",
      size_bytes: 18400000,
      duration_seconds: 92,
      language: "en",
      status: "completed",
      progress: 1,
      created_at: now,
      updated_at: now,
      error: null,
    },
  ];
}

let jobs: Job[] = initial();
try {
  const saved = JSON.parse(localStorage.getItem(key) || "null");
  if (
    Array.isArray(saved) &&
    saved.every(
      (j) =>
        typeof j.id === "string" &&
        typeof j.created_at === "string" &&
        typeof j.filename === "string",
    )
  )
    jobs = saved;
} catch {
  /* Storage may be disabled. Demo still works in memory. */
}

function save() {
  try {
    localStorage.setItem(key, JSON.stringify(jobs));
  } catch {
    /* memory only */
  }
}
function get(id: string) {
  const job = jobs.find((j) => j.id === id);
  if (!job) throw new Error("Демонстрационная запись не найдена.");
  return job;
}

function advance() {
  let running = jobs.find(
    (j) => j.status === "preparing" || j.status === "transcribing",
  );
  if (!running) {
    running = [...jobs].reverse().find((j) => j.status === "queued");
    if (running) {
      running.status = "preparing";
      running.updated_at = new Date().toISOString();
    }
  }
  if (running) {
    const seconds = (Date.now() - Date.parse(running.updated_at)) / 1000;
    running.status =
      seconds >= 18 ? "completed" : seconds >= 3 ? "transcribing" : "preparing";
    running.progress =
      running.status === "completed"
        ? 1
        : running.status === "transcribing"
          ? Math.min(0.99, (seconds - 3) / 15)
          : null;
  }
  save();
}

export const demoApi: JobsApi = {
  async list() {
    advance();
    return structuredClone(jobs);
  },
  async create(file, progress, signal) {
    if (signal.aborted) throw new DOMException("Aborted", "AbortError");
    progress(100);
    const now = new Date().toISOString();
    const job: Job = {
      id: crypto.randomUUID(),
      filename: file.name,
      size_bytes: file.size,
      duration_seconds: 92,
      language: "en",
      status: "queued",
      progress: null,
      created_at: now,
      updated_at: now,
      error: null,
    };
    jobs.unshift(job);
    save();
    return structuredClone(job);
  },
  async transcript(id) {
    get(id);
    return structuredClone(example);
  },
  async cancel(id) {
    const job = get(id);
    if (isActive(job)) {
      job.status = "cancelled";
      job.progress = null;
      save();
    }
    return structuredClone(job);
  },
  async retry(id) {
    const job = get(id);
    job.status = "queued";
    job.progress = null;
    job.error = null;
    job.updated_at = new Date().toISOString();
    save();
    return structuredClone(job);
  },
  async remove(id) {
    jobs = jobs.filter((j) => j.id !== id);
    save();
  },
};
