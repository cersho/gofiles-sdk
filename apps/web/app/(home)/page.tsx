import { ArrowRight, Check, GitPullRequestCreate } from "lucide-react";
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
  },
  {
    href: "/adapters/r2",
    logo: "https://logos.lndev.me/logos/cloudflare.svg",
    name: "Cloudflare R2",
    short: "R2",
  },
  {
    href: "/adapters/vercel-blob",
    logo: "https://logos.lndev.me/logos/vercel.svg",
    name: "Vercel Blob",
    short: "VB",
  },
  {
    href: "/adapters/appwrite",
    logo: "https://logos.lndev.me/logos/appwrite.svg",
    name: "Appwrite",
    short: "AW",
  },
  {
    href: "/adapters/supabase",
    logo: "https://logos.lndev.me/logos/supabase.svg",
    name: "Supabase",
    short: "SB",
  },
  {
    href: "/adapters/digitalocean-spaces",
    logo: "https://logos.lndev.me/logos/digital-ocean.svg",
    name: "DigitalOcean Spaces",
    short: "DO",
  },
  {
    href: "/adapters/s3-compatible",
    logo: "https://logos.lndev.me/logos/amazon-s3.svg",
    name: "S3-compatible",
    short: "S3",
  },
  {
    href: "/adapters/fs",
    logo: "https://logos.lndev.me/logos/files.svg",
    name: "Filesystem",
    short: "FS",
  },
  {
    href: "/adapters/memory",
    logo: "https://logos.lndev.me/logos/files.svg",
    name: "Memory",
    short: "MEM",
  },
  {
    href: "/adapters/uploadthing",
    logo: "https://uploadthing.com/UploadThing-Logo.svg",
    name: "UploadThing",
    short: "UT",
  },
];

const AdapterMarquee = ({ items }: { items: typeof adapters }) => (
  <div className="flex w-max animate-[marquee_42s_linear_infinite] items-center motion-reduce:animate-none">
    {[0, 1].map((group) => (
      <div
        aria-hidden={group === 1}
        className="flex shrink-0 items-center gap-4 px-2"
        key={group}
      >
        {items.map((adapter) => (
          <Link
            className="group flex h-18 min-w-56 items-center gap-4 rounded-xl bg-background px-4 shadow-[var(--shadow-border)] transition-[background-color,box-shadow] duration-150 ease-out hover:bg-[#00add8]/5 hover:shadow-[var(--shadow-border-hover)] focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-muted/25 dark:hover:bg-muted/40"
            href={adapter.href}
            key={`${group}-${adapter.name}`}
            tabIndex={group === 1 ? -1 : undefined}
          >
            <span className="grid size-12 shrink-0 place-items-center rounded-lg bg-white text-[11px] font-semibold text-[#007a99] shadow-[var(--shadow-border)]">
              {"logo" in adapter && adapter.logo ? (
                <img
                  alt=""
                  className="max-h-7 max-w-7 object-contain transition-transform duration-200 ease-out group-hover:scale-105 dark:outline dark:outline-1 dark:-outline-offset-1 dark:outline-white/10"
                  height="28"
                  loading="lazy"
                  src={adapter.logo}
                  width="28"
                />
              ) : (
                adapter.short
              )}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium text-foreground">
                {adapter.name}
              </span>
              <span className="mt-1 block h-px w-8 bg-[#00add8]/45 transition-[width] duration-200 ease-out group-hover:w-12" />
            </span>
            <ArrowRight className="size-3.5 shrink-0 text-[#0087a8] opacity-0 transition-[opacity,transform] duration-200 ease-out group-hover:translate-x-0.5 group-hover:opacity-100" />
          </Link>
        ))}
      </div>
    ))}
  </div>
);

const features = [
  {
    eyebrow: "Operations",
    text: "Upload, download, list, move, copy, search, and sign URLs with one client.",
  },
  {
    eyebrow: "Bodies",
    text: "Use Go-native strings, byte slices, readers, files, and custom read closers.",
  },
  {
    eyebrow: "Adapters",
    text: "Keep provider-specific details at the adapter boundary.",
  },
  {
    eyebrow: "Controls",
    text: "Add prefixes, read-only mode, retries, hooks, middleware, and transfers.",
  },
];

const Home = () => (
  <main className="overflow-hidden">
    <section className="border-b border-dotted">
      <div className="mx-auto flex min-h-[calc(100svh-18rem)] max-w-5xl flex-col items-center justify-center px-6 py-16 text-center sm:py-20">
        <div className="motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-bottom-3 motion-safe:duration-700">
          <p className="font-mono text-xs tracking-wide text-[#0087a8] uppercase">
            Go Files SDK
          </p>
          <h1 className="mx-auto mt-5 max-w-[14ch] text-[3rem]/[0.98] font-medium tracking-tight text-balance text-foreground sm:text-7xl lg:text-8xl">
            Store files with one Go client.
          </h1>
          <p className="mx-auto mt-7 max-w-[58ch] text-base leading-relaxed text-pretty text-muted-foreground sm:text-xl">
            Upload, download, list, move, copy, and sign files across object
            storage providers without rewriting your service code.
          </p>
          <div className="mt-10 flex flex-wrap items-center justify-center gap-3">
            <Button asChild size="lg">
              <Link href="/overview">
                Read the docs
                <ArrowRight data-icon="inline-end" />
              </Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <Link href="/installation">
                Installation
                <ArrowRight data-icon="inline-end" />
              </Link>
            </Button>
          </div>
          <HeroInstallCopy />
        </div>
      </div>
    </section>

    <section className="relative border-b border-dotted bg-muted/20 py-16 sm:py-20">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-[#00add8]/45 to-transparent" />
      <div className="mx-auto grid max-w-6xl gap-8 px-6 lg:grid-cols-[0.9fr_1.1fr] lg:items-end">
        <div className="max-w-2xl">
          <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
            Supported adapters
          </p>
          <h2 className="mt-4 max-w-[18ch] text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
            Bring one client to every storage backend.
          </h2>
        </div>
        <div className="grid gap-5 lg:justify-self-end">
          <p className="max-w-[54ch] text-base leading-relaxed text-pretty text-muted-foreground">
            Use cloud buckets, local disk, memory storage, and managed blob
            platforms behind one Go interface.
          </p>
          <div className="flex flex-wrap gap-2 font-mono text-[11px] tracking-wide text-muted-foreground uppercase">
            <span className="rounded-full bg-background px-3 py-1.5 shadow-[var(--shadow-border)]">
              <span className="tabular-nums">{adapters.length}</span> adapters
            </span>
            <span className="rounded-full bg-background px-3 py-1.5 shadow-[var(--shadow-border)]">
              Same API
            </span>
            <span className="rounded-full bg-background px-3 py-1.5 shadow-[var(--shadow-border)]">
              Provider-native setup
            </span>
          </div>
        </div>
      </div>
      <div className="mx-auto mt-12 max-w-6xl px-6">
        <div className="overflow-hidden rounded-xl bg-background/65 py-7 shadow-[var(--shadow-border)] [-webkit-mask-image:linear-gradient(to_right,transparent,#000_8%,#000_92%,transparent)] [mask-image:linear-gradient(to_right,transparent,#000_8%,#000_92%,transparent)]">
          <AdapterMarquee items={adapters} />
        </div>
      </div>
    </section>

    <section className="border-b border-dotted">
      <div className="mx-auto grid max-w-6xl gap-12 px-6 py-20 sm:py-24">
        <div className="grid gap-8 lg:grid-cols-[0.8fr_1.2fr] lg:items-end">
          <div className="max-w-2xl">
            <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
              Same calls
            </p>
            <h2 className="mt-4 max-w-[18ch] text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
              Swap storage without rewriting the file flow.
            </h2>
          </div>
          <div className="grid gap-5 lg:justify-self-end">
            <p className="max-w-[56ch] text-base leading-relaxed text-pretty text-muted-foreground">
              Initialize the provider once. Upload, download, list, and sign
              files through the same client surface everywhere else.
            </p>
            <div className="flex flex-wrap gap-2 font-mono text-[11px] tracking-wide text-muted-foreground uppercase">
              <span className="rounded-full bg-muted/35 px-3 py-1.5 shadow-[var(--shadow-border)]">
                One client
              </span>
              <span className="rounded-full bg-muted/35 px-3 py-1.5 shadow-[var(--shadow-border)]">
                Typed options
              </span>
              <span className="rounded-full bg-muted/35 px-3 py-1.5 shadow-[var(--shadow-border)]">
                Provider setup only
              </span>
            </div>
          </div>
        </div>
        <div className="relative">
          <div className="pointer-events-none absolute inset-x-8 -top-px h-px bg-gradient-to-r from-transparent via-[#00add8]/55 to-transparent" />
          <div className="rounded-2xl bg-muted/20 p-2 shadow-[var(--shadow-border)]">
            <ProviderCodeSwitcher />
          </div>
        </div>
      </div>
    </section>

    <section className="border-b border-dotted bg-muted/20">
      <div className="mx-auto grid max-w-6xl gap-8 px-6 py-16 sm:py-20 lg:grid-cols-[1fr_auto] lg:items-center">
        <div>
          <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
            Request adapter
          </p>
          <h2 className="mt-4 max-w-[18ch] text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
            Missing a storage provider?
          </h2>
          <p className="mt-5 max-w-[58ch] text-base leading-relaxed text-pretty text-muted-foreground">
            Open an issue with the provider name, API docs, and the operations
            your app needs. Adapter requests help prioritize what lands next.
          </p>
        </div>
        <div className="flex flex-col gap-3 sm:flex-row lg:flex-col lg:items-stretch">
          <Button asChild size="lg">
            <Link
              href="https://github.com/cersho/gofiles-sdk/issues/new?title=Adapter%20request%3A%20&labels=adapter"
              rel="noreferrer"
              target="_blank"
            >
              <GitPullRequestCreate data-icon="inline-start" />
              Request on GitHub
              <ArrowRight data-icon="inline-end" />
            </Link>
          </Button>
          <Button asChild size="lg" variant="outline">
            <Link href="/adapters">
              View adapters
              <ArrowRight data-icon="inline-end" />
            </Link>
          </Button>
        </div>
      </div>
    </section>

    <section className="border-b border-dotted">
      <div className="mx-auto grid max-w-6xl gap-12 px-6 py-20 sm:py-24 lg:grid-cols-[0.82fr_1.18fr]">
        <div className="lg:sticky lg:top-24 lg:self-start">
          <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
            What stays consistent
          </p>
          <h2 className="mt-4 max-w-[18ch] text-4xl font-medium tracking-tight text-balance text-foreground sm:text-5xl">
            A small storage surface for Go services.
          </h2>
          <p className="mt-5 max-w-[44ch] text-base leading-relaxed text-pretty text-muted-foreground">
            Keep storage code boring: one client, typed options, predictable
            errors, and escape hatches for provider-native behavior.
          </p>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {features.map((feature, index) => (
            <div
              className="group min-h-48 rounded-xl bg-background p-5 shadow-[var(--shadow-border)] transition-[box-shadow] duration-150 ease-out hover:shadow-[var(--shadow-border-hover)]"
              key={feature.eyebrow}
            >
              <div className="flex items-center justify-between">
                <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
                  {feature.eyebrow}
                </p>
                <span className="font-mono text-xs tabular-nums text-[#0087a8]">
                  0{index + 1}
                </span>
              </div>
              <p className="mt-12 text-lg leading-relaxed text-pretty text-foreground">
                {feature.text}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>

    <section className="relative">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-[#00add8]/60 to-transparent" />
      <div className="mx-auto grid max-w-6xl gap-10 px-6 py-20 sm:py-24 lg:grid-cols-[1.05fr_0.95fr] lg:items-center">
        <div className="max-w-2xl">
          <p className="font-mono text-xs tracking-wide text-muted-foreground uppercase">
            Start here
          </p>
          <h2 className="mt-4 max-w-[20ch] text-4xl font-medium tracking-tight text-balance text-foreground sm:text-6xl">
            Add the module. Pick an adapter. Ship the file flow.
          </h2>
          <p className="mt-6 max-w-[54ch] text-base leading-relaxed text-pretty text-muted-foreground sm:text-lg">
            Start with memory storage, then move to S3, R2, filesystem, or a
            managed blob provider when your app needs it.
          </p>
        </div>
        <div className="rounded-2xl bg-muted/30 p-5 shadow-[var(--shadow-border)]">
          <div className="font-mono text-sm">
            <div className="flex items-center gap-3 border-b border-dotted pb-4">
              <Check className="size-4 text-[#0087a8]" />
              <span>Install the module</span>
            </div>
            <div className="flex items-center gap-3 border-b border-dotted py-4">
              <Check className="size-4 text-[#0087a8]" />
              <span>Choose an adapter</span>
            </div>
            <div className="flex items-center gap-3 pt-4">
              <Check className="size-4 text-[#0087a8]" />
              <span>Keep the same file calls</span>
            </div>
          </div>
          <div className="mt-8 flex flex-wrap gap-3">
            <Button asChild size="lg">
              <Link href="/installation">
                Installation
                <ArrowRight data-icon="inline-end" />
              </Link>
            </Button>
            <Button asChild size="lg" variant="outline">
              <Link href="/adapters">
                View adapters
                <ArrowRight data-icon="inline-end" />
              </Link>
            </Button>
          </div>
        </div>
      </div>
    </section>
  </main>
);

export default Home;
