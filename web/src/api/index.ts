import type { JobsApi } from "../types/job";
import { httpApi } from "./http";

export const isDemo = __DEMO__;
// The demo adapter is a separate chunk and is not included in normal builds.
export const api: JobsApi = __DEMO__
  ? (await import("./demo")).demoApi
  : httpApi;
