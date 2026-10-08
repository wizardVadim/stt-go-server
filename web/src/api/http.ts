import {
  statuses,
  type Job,
  type JobsApi,
  type Transcript,
} from "../types/job";

const base = "/api";
export class ApiError extends Error {
  constructor(
    message: string,
    public status = 0,
  ) {
    super(message);
  }
}

function message(body: unknown, status: number): string {
  const error = (body as { error?: { message?: unknown } })?.error;
  if (typeof error?.message === "string") return error.message;
  if (status === 413)
    return "Файл слишком большой для сервера. Попробуйте файл меньшего размера.";
  if (status === 422)
    return "Не удалось прочитать запись. Проверьте формат и длительность — не более 10 минут.";
  if (status === 409)
    return "Состояние записи изменилось. Обновите список и повторите действие.";
  return "Не удалось выполнить запрос. Проверьте подключение и попробуйте ещё раз.";
}

function parseJob(value: unknown): Job {
  const j = value as Job;
  if (
    !j ||
    typeof j.id !== "string" ||
    !j.id ||
    typeof j.filename !== "string" ||
    !statuses.includes(j.status) ||
    j.language !== "en" ||
    typeof j.size_bytes !== "number" ||
    !Number.isFinite(j.size_bytes) ||
    j.size_bytes < 0 ||
    (j.duration_seconds !== null &&
      (typeof j.duration_seconds !== "number" ||
        !Number.isFinite(j.duration_seconds) ||
        j.duration_seconds <= 0)) ||
    (j.progress !== null &&
      (typeof j.progress !== "number" ||
        !Number.isFinite(j.progress) ||
        j.progress < 0 ||
        j.progress > 1)) ||
    !Number.isFinite(Date.parse(j.created_at)) ||
    !Number.isFinite(Date.parse(j.updated_at)) ||
    (j.error !== null && typeof j.error !== "string")
  )
    throw new ApiError("Сервер вернул некорректные данные записи.");
  return j;
}

function parseTranscript(value: unknown): Transcript {
  const t = value as Transcript;
  if (
    !t ||
    t.language !== "en" ||
    typeof t.text !== "string" ||
    !Array.isArray(t.segments) ||
    t.segments.some(
      (s) =>
        !s ||
        typeof s.text !== "string" ||
        !Number.isFinite(s.start) ||
        !Number.isFinite(s.end) ||
        s.start < 0 ||
        s.end < s.start,
    )
  ) {
    throw new ApiError("Сервер вернул некорректную расшифровку.");
  }
  return t;
}

async function request(
  path: string,
  options: RequestInit = {},
): Promise<unknown> {
  const timeout = AbortSignal.timeout(15000);
  let response: Response;
  try {
    response = await fetch(base + path, {
      ...options,
      signal: options.signal
        ? AbortSignal.any([options.signal, timeout])
        : timeout,
      headers: { Accept: "application/json", ...options.headers },
    });
  } catch (error) {
    if (options.signal?.aborted) throw error;
    throw new ApiError(
      "Нет связи с сервером. Сохранённые записи появятся после подключения.",
    );
  }
  const body =
    response.status === 204 ? null : await response.json().catch(() => null);
  if (!response.ok)
    throw new ApiError(message(body, response.status), response.status);
  return body;
}

const path = (id: string) => `/jobs/${encodeURIComponent(id)}`;
export const httpApi: JobsApi = {
  async list(signal) {
    const response = (await request("/jobs", { signal })) as {
      jobs?: unknown[];
    };
    if (!response || !Array.isArray(response.jobs))
      throw new ApiError("Сервер вернул некорректный список записей.");
    return response.jobs.map(parseJob);
  },
  create(file, progress, signal) {
    return new Promise((resolve, reject) => {
      if (signal.aborted)
        return reject(new DOMException("Aborted", "AbortError"));
      const xhr = new XMLHttpRequest();
      const abort = () => xhr.abort();
      signal.addEventListener("abort", abort, { once: true });
      xhr.open("POST", base + "/jobs");
      xhr.timeout = 30 * 60 * 1000;
      xhr.setRequestHeader("Accept", "application/json");
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable)
          progress(Math.round((event.loaded / event.total) * 100));
      };
      xhr.onloadend = () => signal.removeEventListener("abort", abort);
      xhr.onabort = () => reject(new DOMException("Aborted", "AbortError"));
      xhr.onerror = () =>
        reject(
          new ApiError(
            "Соединение прервалось. Проверьте список: сервер мог успеть принять файл.",
          ),
        );
      xhr.ontimeout = () =>
        reject(
          new ApiError(
            "Загрузка заняла слишком много времени. Проверьте список перед повторной отправкой.",
          ),
        );
      xhr.onload = () => {
        let body: unknown;
        try {
          body = JSON.parse(xhr.responseText);
        } catch {
          body = null;
        }
        if (xhr.status < 200 || xhr.status >= 300)
          return reject(new ApiError(message(body, xhr.status), xhr.status));
        try {
          resolve(parseJob(body));
        } catch (error) {
          reject(error);
        }
      };
      const data = new FormData();
      data.append("file", file);
      data.append("language", "en");
      xhr.send(data);
    });
  },
  async transcript(id, signal) {
    return parseTranscript(await request(`${path(id)}/transcript`, { signal }));
  },
  async cancel(id) {
    return parseJob(await request(`${path(id)}/cancel`, { method: "POST" }));
  },
  async retry(id) {
    return parseJob(await request(`${path(id)}/retry`, { method: "POST" }));
  },
  async remove(id) {
    await request(path(id), { method: "DELETE" });
  },
};
