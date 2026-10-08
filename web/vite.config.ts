import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig(({ mode }) => {
  // Only web/.env files are read. Repository SSH credentials are never loaded.
  const env = loadEnv(mode, process.cwd(), "STT_");
  return {
    plugins: [vue()],
    define: { __DEMO__: JSON.stringify(mode === "demo") },
    server: {
      port: 5174,
      strictPort: true,
      proxy: {
        "/api": {
          target: env.STT_API_PROXY || "http://127.0.0.1:8080",
          changeOrigin: false,
        },
      },
    },
  };
});
