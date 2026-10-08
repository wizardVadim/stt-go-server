export const MAX_DURATION = 600;
export const extensions = [
  "mp3",
  "wav",
  "m4a",
  "mp4",
  "webm",
  "ogg",
  "flac",
  "mov",
  "mkv",
  "aac",
  "opus",
];
export const acceptMedia = extensions.map((ext) => `.${ext}`).join(",");

export function checkFile(file: File): string | null {
  if (file.size === 0)
    return "Файл пустой. Выберите аудио или видео с записью.";
  const extension = file.name.split(".").pop()?.toLowerCase() || "";
  if (!extensions.includes(extension))
    return "Этот формат не поддерживается. Выберите MP3, WAV, M4A, MP4, WebM, OGG, FLAC, MOV, MKV, AAC или Opus.";
  return null;
}

/** A browser cannot decode every format FFmpeg supports. Null defers to the server. */
export function readDuration(file: File): Promise<number | null> {
  return new Promise((resolve) => {
    const media = document.createElement("video");
    const url = URL.createObjectURL(file);
    const timer = window.setTimeout(() => finish(null), 8000);
    let finished = false;
    function finish(value: number | null) {
      if (finished) return;
      finished = true;
      window.clearTimeout(timer);
      media.onloadedmetadata = null;
      media.onerror = null;
      media.removeAttribute("src");
      media.load();
      URL.revokeObjectURL(url);
      resolve(value);
    }
    media.preload = "metadata";
    media.onloadedmetadata = () =>
      finish(
        Number.isFinite(media.duration) && media.duration > 0
          ? media.duration
          : null,
      );
    media.onerror = () => finish(null);
    media.src = url;
  });
}
