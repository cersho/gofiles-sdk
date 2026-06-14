import {
  ArrowRight,
  Check,
  Copy,
  Database,
  GitPullRequestCreate,
  HardDrive,
  Lock,
  RefreshCw,
  Route,
  Search,
  ShieldCheck,
  Upload,
} from "lucide-react";
import type { Metadata } from "next";
import Link from "next/link";

import { HeroInstallCopy } from "@/components/hero-install-copy";
import { ProviderCodeSwitcher } from "@/components/provider-code-switcher";
import { Button } from "@/components/ui/button";

export const metadata: Metadata = {
  alternates: {
    canonical: "/",
  },
};

const adapters = [
  {
    href: "/adapters/s3",
    logo: "https://logos.lndev.me/logos/aws.svg",
    name: "Amazon S3",
    short: "S3",
    type: "Object storage",
  },
  {
    href: "/adapters/r2",
    logo: "https://logos.lndev.me/logos/cloudflare.svg",
    name: "Cloudflare R2",
    short: "R2",
    type: "S3-compatible",
  },
  {
    href: "/adapters/vercel-blob",
    logo: "https://logos.lndev.me/logos/vercel.svg",
    name: "Vercel Blob",
    short: "VB",
    type: "Managed blobs",
  },
  {
    href: "/adapters/appwrite",
    logo: "https://logos.lndev.me/logos/appwrite.svg",
    name: "Appwrite",
    short: "AW",
    type: "Platform storage",
  },
  {
    href: "/adapters/supabase",
    logo: "https://logos.lndev.me/logos/supabase.svg",
    name: "Supabase",
    short: "SB",
    type: "Database platform",
  },
  {
    href: "/adapters/digitalocean-spaces",
    logo: "https://logos.lndev.me/logos/digital-ocean.svg",
    name: "DigitalOcean Spaces",
    short: "DO",
    type: "Object storage",
  },
  {
    href: "/adapters/s3-compatible",
    logo: "https://logos.lndev.me/logos/amazon-s3.svg",
    name: "S3-compatible",
    short: "S3",
    type: "Custom endpoint",
  },
  {
    href: "/adapters/fs",
    logo: "https://logos.lndev.me/logos/files.svg",
    name: "Filesystem",
    short: "FS",
    type: "Local disk",
  },
  {
    href: "/adapters/memory",
    logo: "https://logos.lndev.me/logos/files.svg",
    name: "Memory",
    short: "MEM",
    type: "Tests and dev",
  },
  {
    href: "/adapters/uploadthing",
    logo: "https://uploadthing.com/UploadThing-Logo.svg",
    name: "UploadThing",
    short: "UT",
    type: "Upload platform",
  },
];

const clientCalls = [
  "Upload",
  "Download",
  "List",
  "Search",
  "Copy",
  "Move",
  "Sign",
];

const workflow = [
  {
    icon: Upload,
    label: "Accept a Go body",
    text: "Reader, file, bytes, string, or a custom closer.",
  },
  {
    icon: Route,
    label: "Pass through one client",
    text: "Options, hooks, middleware, retries, and prefixes stay in one place.",
  },
  {
    icon: Database,
    label: "Land in any backend",
    text: "Cloud bucket, local disk, memory storage, or managed blob platform.",
  },
];

const controls = [
  {
    icon: ShieldCheck,
    label: "Read-only mode",
    text: "Protect environments that should never mutate stored files.",
  },
  {
    icon: RefreshCw,
    label: "Retries and timeouts",
    text: "Keep transient provider failures away from app code.",
  },
  {
    icon: Search,
    label: "List and search",
    text: "Expose object discovery without binding callers to provider APIs.",
  },
  {
    icon: Lock,
    label: "Signed URLs",
    text: "Generate temporary access at the same call site everywhere.",
  },
];

const Home = () => (
  <main className="overflow-hidden bg-background">
    <section className="relative border-b border-dotted">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-[#00add8]/25" />
      <div className="mx-auto flex max-w-7xl flex-col items-center justify-center px-6 py-14 text-center sm:min-h-[calc(100svh-14rem)] sm:py-16">
        <div className="mx-auto max-w-5xl">
          <p className="landing-reveal font-mono text-xs tracking-wide text-[#0087a8] uppercase">
            Go Files SDK
          </p>
          <h1 className="landing-reveal landing-reveal-delay-1 mx-auto mt-5 max-w-[13ch] text-[3.1rem]/[0.94] font-medium tracking-tight text-balance text-foreground sm:text-[5.1rem] lg:text-[6.3rem]">
            Unified{" "}
            <span className="sr-only">Files</span>
            <span aria-hidden="true" className="inline-flex align-[-0.08em]">
              <img
                alt=""
                className="h-[0.54em] w-auto object-contain"
                height="64"
                src="https://logos.lndev.me/logos/files.svg"
                width="64"
              />
            </span>{" "}
            API for{" "}
            <span className="sr-only">Go</span>
            <span aria-hidden="true" className="inline-flex align-[-0.07em]">
              <img
                alt=""
                className="h-[0.42em] w-auto object-contain"
                height="64"
                src="https://logos.lndev.me/logos/go.svg"
                width="154"
              />
            </span>{" "}
            apps.
          </h1>
          <p className="landing-reveal landing-reveal-delay-2 mx-auto mt-6 max-w-[50ch] text-base leading-relaxed text-pretty text-muted-foreground sm:text-lg">
            A small Go storage layer for uploads, downloads, lists, and signed
            URLs across local, memory, S3, R2, and blob adapters.
          </p>
          <div className="landing-reveal landing-reveal-delay-3 mt-8 flex flex-wrap items-center justify-center gap-3">
            <Button asChild size="lg">
              <Link href="/overview">
                Read the docs
                <ArrowRight data-icon="inline-end" />
              </Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <Link href="/installation">
                Install
                <ArrowRight data-icon="inline-end" />
              </Link>
            </Button>
          </div>
          <div className="landing-reveal landing-reveal-delay-4">
            <HeroInstallCopy />
          </div>
        </div>

      </div>
    </section>

    <section className="bg-muted/20">
      <div className="mx-auto grid max-w-7xl gap-10 px-6 py-16 sm:py-20">
        <div className="landing-reveal flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
          <div className="max-w-3xl">
            <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
              Adapter directory
            </p>
            <h2 className="mt-4 text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
              Choose storage at the edge of your system.
            </h2>
          </div>
          <Button asChild size="lg" variant="outline">
            <Link href="/adapters">
              View adapters
              <ArrowRight data-icon="inline-end" />
            </Link>
          </Button>
        </div>
        <div className="grid gap-px overflow-hidden rounded-2xl bg-border shadow-[var(--shadow-border)] sm:grid-cols-2 lg:grid-cols-5">
          {adapters.map((adapter) => (
            <Link
              className="group flex min-h-32 flex-col justify-between bg-background p-4 transition-[background-color] duration-150 ease-out hover:bg-[#00add8]/5 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
              href={adapter.href}
              key={adapter.name}
            >
              <div className="flex items-start justify-between gap-3">
                <span className="grid size-11 place-items-center rounded-xl bg-background shadow-[var(--shadow-border)]">
                  {"logo" in adapter && adapter.logo ? (
                    <img
                      alt=""
                      className="max-h-7 max-w-7 object-contain"
                      height="28"
                      loading="lazy"
                      src={adapter.logo}
                      width="28"
                    />
                  ) : (
                    adapter.short
                  )}
                </span>
                <ArrowRight className="size-4 text-[#0087a8] opacity-0 transition-[opacity,transform] duration-150 ease-out group-hover:translate-x-0.5 group-hover:opacity-100" />
              </div>
              <div>
                <p className="font-medium text-foreground">{adapter.name}</p>
                <p className="mt-1 font-mono text-[11px] tracking-wide text-muted-foreground uppercase">
                  {adapter.type}
                </p>
              </div>
            </Link>
          ))}
        </div>
      </div>
    </section>

    <section className="border-y border-dotted">
      <div className="mx-auto grid max-w-7xl gap-10 px-6 py-16 sm:py-20">
        <div className="landing-reveal flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
          <div className="max-w-3xl">
            <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
              Storage shape
            </p>
            <h2 className="mt-4 text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
              One path from Go data to storage.
            </h2>
          </div>
          <p className="max-w-[42ch] text-sm leading-relaxed text-pretty text-muted-foreground sm:text-base">
            The SDK keeps inputs, operations, and provider setup in separate
            places so app code can stay direct.
          </p>
        </div>
        <div className="grid gap-px overflow-hidden rounded-2xl bg-border shadow-[var(--shadow-border)] lg:grid-cols-3">
          {workflow.map((item, index) => (
            <div
              className="landing-reveal min-h-56 bg-background p-5"
              key={item.label}
            >
              <div className="flex items-start justify-between gap-4">
                <div className="grid size-11 place-items-center rounded-xl bg-muted/35">
                  <item.icon className="size-5 text-[#0087a8]" />
                </div>
                <span className="font-mono text-xs tabular-nums text-muted-foreground">
                  0{index + 1}
                </span>
              </div>
              <p className="mt-10 text-lg font-medium text-foreground">
                {item.label}
              </p>
              <p className="mt-2 text-sm leading-relaxed text-pretty text-muted-foreground">
                {item.text}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>

    <section>
      <div className="mx-auto grid max-w-6xl gap-8 px-6 py-16 sm:py-20">
        <div className="landing-reveal max-w-3xl">
          <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
            Same calls
          </p>
          <h2 className="mt-4 text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
            One file flow. Different adapters.
          </h2>
          <p className="mt-5 max-w-[58ch] text-base leading-relaxed text-pretty text-muted-foreground">
            Swap provider setup at the boundary; keep the upload and access
            calls in app code unchanged.
          </p>
        </div>
        <div className="landing-reveal landing-reveal-delay-1 min-w-0">
          <ProviderCodeSwitcher />
        </div>
      </div>
    </section>

    <section className="border-b border-dotted">
      <div className="mx-auto grid max-w-7xl gap-10 px-6 py-16 sm:py-20 lg:grid-cols-[1fr_1fr]">
        <div className="landing-reveal max-w-2xl">
          <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
            Runtime controls
          </p>
          <h2 className="mt-4 text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
            Put storage policy beside storage calls.
          </h2>
          <p className="mt-5 text-base leading-relaxed text-pretty text-muted-foreground">
            Prefixes, hooks, middleware, retries, timeouts, and read-only mode
            stay in the SDK layer so app code can stay direct.
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {controls.map((control) => (
            <div
              className="landing-reveal min-h-44 rounded-2xl bg-background p-5 shadow-[var(--shadow-border)] transition-[translate,box-shadow] duration-150 ease-out hover:-translate-y-0.5 hover:shadow-[var(--shadow-border-hover)]"
              key={control.label}
            >
              <control.icon className="size-5 text-[#0087a8]" />
              <p className="mt-8 font-medium text-foreground">
                {control.label}
              </p>
              <p className="mt-2 text-sm leading-relaxed text-pretty text-muted-foreground">
                {control.text}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>

    <section>
      <div className="mx-auto grid max-w-7xl gap-10 px-6 py-16 sm:py-20 lg:grid-cols-[1fr_auto] lg:items-center">
        <div className="landing-reveal max-w-3xl">
          <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
            Start
          </p>
          <h2 className="mt-4 text-4xl font-medium tracking-tight text-balance text-foreground sm:text-6xl">
            Add one module. Move storage decisions later.
          </h2>
          <p className="mt-5 max-w-[58ch] text-base leading-relaxed text-pretty text-muted-foreground sm:text-lg">
            Start with memory or filesystem in development, then point the same
            file flow at S3, R2, or a managed provider in production.
          </p>
        </div>
        <div className="landing-reveal landing-reveal-delay-1 grid gap-3 sm:min-w-80">
          <Button asChild size="lg">
            <Link href="/installation">
              Installation
              <ArrowRight data-icon="inline-end" />
            </Link>
          </Button>
          <Button asChild size="lg" variant="outline">
            <Link
              href="https://github.com/cersho/gofiles-sdk/issues/new?title=Adapter%20request%3A%20&labels=adapter"
              rel="noreferrer"
              target="_blank"
            >
              <GitPullRequestCreate data-icon="inline-start" />
              Request adapter
            </Link>
          </Button>
          <div className="mt-3 grid gap-3 rounded-2xl bg-muted/25 p-4 font-mono text-xs text-muted-foreground shadow-[var(--shadow-border)]">
            <div className="flex items-center gap-3">
              <Check className="size-4 text-[#0087a8]" />
              <span>Typed Go options</span>
            </div>
            <div className="flex items-center gap-3">
              <Copy className="size-4 text-[#0087a8]" />
              <span>Same operation names</span>
            </div>
            <div className="flex items-center gap-3">
              <HardDrive className="size-4 text-[#0087a8]" />
              <span>Provider escape hatches</span>
            </div>
          </div>
        </div>
      </div>
    </section>
  </main>
);

export default Home;
