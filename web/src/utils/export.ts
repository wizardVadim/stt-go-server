import type { Transcript } from "../types/job";

function timestamp(seconds: number): string {
  const ms = Math.max(0, Math.round(seconds * 1000));
  return `${String(Math.floor(ms / 3600000)).padStart(2, "0")}:${String(Math.floor(ms / 60000) % 60).padStart(2, "0")}:${String(Math.floor(ms / 1000) % 60).padStart(2, "0")},${String(ms % 1000).padStart(3, "0")}`;
}

export function downloadTranscript(
  transcript: Transcript,
  filename: string,
  format: "txt" | "srt" | "json",
) {
  const contents =
    format === "txt"
      ? transcript.text
      : format === "json"
        ? JSON.stringify(transcript, null, 2)
        : transcript.segments
            .map(
              (s, i) =>
                `${i + 1}\n${timestamp(s.start)} --> ${timestamp(s.end)}\n${s.text.trim()}\n`,
            )
            .join("\n");
  const url = URL.createObjectURL(
    new Blob([contents], {
      type:
        format === "json"
          ? "application/json;charset=utf-8"
          : "text/plain;charset=utf-8",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = `${filename.replace(/\.[^.]+$/, "").replace(/[\\/:*?"<>|]/g, "_")}.${format}`;
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
