import { codeToHtml, type ThemeRegistration } from "shiki";

import { ProviderCodeSwitcherClient } from "@/components/provider-code-switcher-client";
import {
  goBlueDarkTheme,
  goBlueLightTheme,
} from "@/lib/shiki-themes";

const samples = [
  {
    detail: "No credentials, useful for tests and local development.",
    href: "/adapters/memory",
    id: "memory",
    label: "Memory",
    code: `client := files.MustNew(files.Options{
    Adapter: memory.New(memory.Options{}),
})`,
  },
  {
    detail: "Use AWS S3 buckets with the same client calls.",
    href: "/adapters/s3",
    id: "s3",
    label: "S3",
    code: `client := files.MustNew(files.Options{
    Adapter: s3.New(s3.Options{
        Bucket: "app-files",
        Region: "us-east-1",
    }),
})`,
  },
  {
    detail: "Point the adapter at Cloudflare's S3-compatible object storage.",
    href: "/adapters/r2",
    id: "r2",
    label: "R2",
    code: `client := files.MustNew(files.Options{
    Adapter: r2.New(r2.Options{
        Bucket: "app-files",
        AccountID: os.Getenv("R2_ACCOUNT_ID"),
    }),
})`,
  },
  {
    detail: "Store files on disk behind the same storage interface.",
    href: "/adapters/fs",
    id: "fs",
    label: "Filesystem",
    code: `client := files.MustNew(files.Options{
    Adapter: fs.New(fs.Options{
        Root: "./uploads",
    }),
})`,
  },
  {
    detail: "Route uploads through UploadThing without changing call sites.",
    href: "/adapters/uploadthing",
    id: "uploadthing",
    label: "UploadThing",
    code: `client := files.MustNew(files.Options{
    Adapter: uploadthing.New(uploadthing.Options{
        Token: os.Getenv("UPLOADTHING_TOKEN"),
    }),
})`,
  },
];

const operation = `_, err := client.Upload(
    ctx,
    "reports/q1.txt",
    files.StringBody("revenue: 42"),
    files.UploadOptions{ContentType: "text/plain"},
)`;

const highlightCode = (code: string, theme: ThemeRegistration) =>
  codeToHtml(code, {
    lang: "go",
    theme,
  });

export const ProviderCodeSwitcher = async () => {
  const highlightedSamples = await Promise.all(
    samples.map(async (sample) => {
      const code = `${sample.code}\n\n${operation}`;

      return {
        ...sample,
        code,
        highlightedHtmlDark: await highlightCode(code, goBlueDarkTheme),
        highlightedHtmlLight: await highlightCode(code, goBlueLightTheme),
      };
    })
  );

  return <ProviderCodeSwitcherClient samples={highlightedSamples} />;
};
