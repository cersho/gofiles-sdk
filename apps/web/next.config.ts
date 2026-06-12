import { createMDX } from "fumadocs-mdx/next";
import type { NextConfig } from "next";
import path from "node:path";

const toEsbuildPath = (value: string) => value.replaceAll("\\", "/");

const withMDX = createMDX({
  configPath: toEsbuildPath(path.join(process.cwd(), "source.config.ts")),
  outDir: toEsbuildPath(path.join(process.cwd(), ".source")),
});

const nextConfig: NextConfig = {
  redirects: () => [
    {
      destination: "/overview",
      permanent: true,
      source: "/docs",
    },
    {
      destination: "/adapters",
      permanent: false,
      source: "/providers",
    },
  ],
  rewrites: () => [
    {
      destination: "/llms.mdx/:path*",
      source: "/:path*.md",
    },
  ],
};

export default withMDX(nextConfig);
