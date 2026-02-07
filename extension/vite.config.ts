import { defineConfig } from "vite";
import { resolve } from "path";
import { copyFileSync, mkdirSync } from "fs";

function copyStaticFiles() {
  return {
    name: "copy-static-files",
    writeBundle() {
      const distDir = resolve(__dirname, "dist");
      mkdirSync(distDir, { recursive: true });
      copyFileSync(resolve(__dirname, "src/manifest.json"), resolve(distDir, "manifest.json"));
      copyFileSync(resolve(__dirname, "src/popup.html"), resolve(distDir, "popup.html"));
      copyFileSync(resolve(__dirname, "src/popup.css"), resolve(distDir, "popup.css"));
    },
  };
}

export default defineConfig({
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      input: {
        background: resolve(__dirname, "src/background.ts"),
        content: resolve(__dirname, "src/content.ts"),
        popup: resolve(__dirname, "src/popup.ts"),
      },
      output: {
        entryFileNames: "[name].js",
        format: "es",
        // 禁止代码拆分，确保每个入口是独立文件
        manualChunks: undefined,
      },
    },
    minify: false,
  },
  plugins: [copyStaticFiles()],
});
