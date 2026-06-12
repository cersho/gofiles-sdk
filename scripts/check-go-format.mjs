#!/usr/bin/env node
import { existsSync, readdirSync, statSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join } from "node:path";

const root = ".";
const ignoredDirs = new Set([
  ".git",
  ".github",
  ".gocache",
  ".gocache-vet",
  ".gomodcache",
  ".next",
  ".source",
  "apps",
  "dist",
  "node_modules",
  "packages",
]);

const listGoFiles = (dir) => {
  const entries = readdirSync(dir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (ignoredDirs.has(entry.name)) {
        continue;
      }
      files.push(...listGoFiles(path));
      continue;
    }

    if (entry.isFile() && entry.name.endsWith(".go")) {
      files.push(path);
    }
  }

  return files;
};

if (!existsSync(root) || !statSync(root).isDirectory()) {
  console.error(`Missing Go project directory: ${root}`);
  process.exit(1);
}

const files = listGoFiles(root);
if (files.length === 0) {
  console.error(`No Go files found under ${root}`);
  process.exit(1);
}

const result = spawnSync("gofmt", ["-l", ...files], {
  encoding: "utf8",
  stderr: "inherit",
});

if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

const unformatted = result.stdout.trim();
if (unformatted.length > 0) {
  console.error("Go files need formatting:");
  console.error(unformatted);
  process.exit(1);
}
