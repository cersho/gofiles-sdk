import path from "node:path";
import { fileURLToPath } from "node:url";

import { postInstall } from "fumadocs-mdx/next";

const appDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const toEsbuildPath = (value) => value.replaceAll("\\", "/");

process.chdir(appDir);

await postInstall({
  configPath: toEsbuildPath(path.join(appDir, "source.config.ts")),
  outDir: toEsbuildPath(path.join(appDir, ".source")),
});
