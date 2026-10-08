import { expect, test, type Page } from "@playwright/test";
import type { Job, Transcript } from "../src/types/job";

const now = "2026-10-07T12:00:00Z";
const completed: Job = {
  id: "lesson",
  filename: "Learning.mp4",
  size_bytes: 1024,
  duration_seconds: 60,
  language: "en",
  status: "completed",
  progress: 1,
  created_at: now,
  updated_at: now,
  error: null,
};
const transcript: Transcript = {
  language: "en",
  text: 'Every idea deserves a closer look.\n\n<script>alert("unsafe")</script>',
  segments: [
    { start: 0, end: 20, text: "Every idea deserves a closer look." },
    { start: 20, end: 59.5, text: '<script>alert("unsafe")</script>' },
  ],
};

async function mockApi(page: Page, initial: Job[] = [completed]) {
  let jobs = structuredClone(initial);
  const calls: string[] = [];
  await page.route(/\/api\/jobs(?:\/.*)?$/, async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    const method = request.method();
    calls.push(`${method} ${pathname}`);
    if (pathname === "/api/jobs" && method === "GET")
      return route.fulfill({ json: { jobs } });
    if (pathname === "/api/jobs" && method === "POST") {
      expect(request.postDataBuffer()?.toString()).toContain(
        'name="language"\r\n\r\nen',
      );
      const job = {
        ...completed,
        id: "uploaded",
        filename: "sample.wav",
        status: "queued" as const,
        progress: null,
      };
      jobs = [job, ...jobs];
      return route.fulfill({ status: 202, json: job });
    }
    if (pathname.endsWith("/transcript"))
      return route.fulfill({ json: transcript });
    const id = pathname.split("/")[3];
    const job = jobs.find((j) => j.id === id);
    if (pathname.endsWith("/cancel") && job) {
      job.status = "cancelled";
      job.progress = null;
      return route.fulfill({ json: job });
    }
    if (pathname.endsWith("/retry") && job) {
      job.status = "queued";
      job.progress = null;
      job.error = null;
      return route.fulfill({ status: 202, json: job });
    }
    if (method === "DELETE") {
      jobs = jobs.filter((j) => j.id !== id);
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({
      status: 404,
      json: { error: { code: "not_found", message: "Не найдено" } },
    });
  });
  return calls;
}

function wav(seconds: number) {
  const samples = Math.ceil(seconds * 8000);
  const bytes = samples * 2;
  const buffer = Buffer.alloc(44 + bytes);
  buffer.write("RIFF");
  buffer.writeUInt32LE(36 + bytes, 4);
  buffer.write("WAVEfmt ", 8);
  buffer.writeUInt32LE(16, 16);
  buffer.writeUInt16LE(1, 20);
  buffer.writeUInt16LE(1, 22);
  buffer.writeUInt32LE(8000, 24);
  buffer.writeUInt32LE(16000, 28);
  buffer.writeUInt16LE(2, 32);
  buffer.writeUInt16LE(16, 34);
  buffer.write("data", 36);
  buffer.writeUInt32LE(bytes, 40);
  return { name: "sample.wav", mimeType: "audio/wav", buffer };
}

test("renders full text safely, changes view, downloads SRT and restores selection", async ({
  page,
}) => {
  await mockApi(page, [
    completed,
    { ...completed, id: "second", filename: "Second.mp3" },
  ]);
  await page.goto("/");
  await expect(page.locator(".transcript-text")).toContainText(
    '<script>alert("unsafe")</script>',
  );
  expect(await page.locator(".transcript-text script").count()).toBe(0);
  await page.getByRole("button", { name: "Таймкоды", exact: true }).click();
  await expect(page.locator(".segment-time").last()).toHaveText("0:20");
  await page.getByLabel("Формат скачивания").selectOption("srt");
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Скачать", exact: true }).click();
  const file = await download;
  expect(file.suggestedFilename()).toBe("Learning.srt");
  const stream = await file.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  expect(Buffer.concat(chunks).toString()).toContain(
    "00:00:20,000 --> 00:00:59,500",
  );
  await page.getByRole("button", { name: /Second.mp3/ }).click();
  await page.reload();
  await expect(page.getByRole("heading", { name: "Second.mp3" })).toBeVisible();
});

test("upload validates English field and moves accepted file into queue", async ({
  page,
}) => {
  const calls = await mockApi(page, []);
  await page.goto("/");
  await page.getByLabel("Выбрать аудио или видео").setInputFiles(wav(1));
  const submit = page.getByRole("button", { name: "Распознать", exact: true });
  await expect(submit).toBeEnabled();
  await submit.click();
  await expect(
    page.getByRole("heading", { name: "Ваша запись в очереди" }),
  ).toBeVisible();
  expect(calls).toContain("POST /api/jobs");
  await expect(page.getByText("Перетащите сюда аудио или видео")).toBeVisible();
});

test("duration is accepted at exactly 600s and rejected above 600s before upload", async ({
  page,
}) => {
  const calls = await mockApi(page, []);
  await page.goto("/");
  const input = page.getByLabel("Выбрать аудио или видео");
  await input.setInputFiles(wav(600));
  await expect(
    page.getByRole("button", { name: "Распознать", exact: true }),
  ).toBeEnabled();
  await input.setInputFiles(wav(600.1));
  await expect(page.getByRole("alert")).toContainText("Максимум — 10 минут");
  await expect(
    page.getByRole("button", { name: "Распознать", exact: true }),
  ).toBeDisabled();
  expect(calls.filter((c) => c === "POST /api/jobs")).toHaveLength(0);
});

test("rejects empty and unsupported uploads", async ({ page }) => {
  await mockApi(page, []);
  await page.goto("/");
  const input = page.getByLabel("Выбрать аудио или видео");
  await input.setInputFiles({
    name: "empty.wav",
    mimeType: "audio/wav",
    buffer: Buffer.alloc(0),
  });
  await expect(page.getByRole("alert")).toContainText("Файл пустой");
  await input.setInputFiles({
    name: "notes.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("test"),
  });
  await expect(page.getByRole("alert")).toContainText(
    "формат не поддерживается",
  );
});

test("cancels, retries and deletes only after confirmation", async ({
  page,
}) => {
  const calls = await mockApi(page, [
    { ...completed, status: "transcribing", progress: 0.42 },
  ]);
  await page.goto("/");
  await expect(page.getByText("42%", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Отменить обработку" }).click();
  await expect(
    page.getByRole("heading", { name: "Обработка отменена" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Повторить", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Ваша запись в очереди" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Отменить обработку" }).click();
  await page
    .getByRole("button", { name: "Удалить запись", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "Оставить" }).click();
  expect(calls.filter((c) => c.startsWith("DELETE"))).toHaveLength(0);
  await page
    .getByRole("button", { name: "Удалить запись", exact: true })
    .click();
  await page.getByRole("button", { name: "Удалить", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Здесь появится текст" }),
  ).toBeVisible();
  expect(calls).toContain("DELETE /api/jobs/lesson");
});

test("offline state does not silently substitute demo records", async ({
  page,
}) => {
  await page.route(/\/api\/jobs(?:\/.*)?$/, (route) => route.abort());
  await page.goto("/");
  await expect(
    page.getByText("Сервер недоступен", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Нет связи с сервером.", { exact: false }),
  ).toBeVisible();
  await expect(page.locator(".job-item")).toHaveCount(0);
  await expect(page.locator(".demo-notice")).toHaveCount(0);
});

test("search, empty results and mobile layout work without horizontal overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await mockApi(page);
  await page.goto("/");
  await page.getByLabel("Поиск записей").fill("missing");
  await expect(page.getByText("Ничего не нашлось")).toBeVisible();
  await page.getByLabel("Поиск записей").clear();
  await expect(page.locator(".job-item")).toHaveCount(1);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
  await expect(
    page.getByRole("heading", { name: "Learning.mp4" }),
  ).toBeVisible();
});

test("server validation errors are displayed without creating a local fake job", async ({
  page,
}) => {
  await mockApi(page, []);
  await page.route("**/api/jobs", async (route) => {
    if (route.request().method() === "POST")
      return route.fulfill({
        status: 422,
        json: {
          error: {
            code: "duration_exceeded",
            message: "Длительность записи превышает 10 минут.",
          },
        },
      });
    return route.fallback();
  });
  await page.goto("/");
  await page.getByLabel("Выбрать аудио или видео").setInputFiles(wav(1));
  await page.getByRole("button", { name: "Распознать", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Длительность записи превышает 10 минут.",
  );
  await expect(page.locator(".job-item")).toHaveCount(0);
});
