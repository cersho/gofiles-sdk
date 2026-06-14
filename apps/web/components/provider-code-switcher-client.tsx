"use client";

import { ArrowRight, Check, Copy } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

interface CodeSample {
  code: string;
  detail: string;
  highlightedHtmlDark: string;
  highlightedHtmlLight: string;
  href: string;
  id: string;
  label: string;
}

interface ProviderCodeSwitcherClientProps {
  samples: CodeSample[];
}

export const ProviderCodeSwitcherClient = ({
  samples,
}: ProviderCodeSwitcherClientProps) => {
  const [active, setActive] = useState(samples[0]);
  const [copied, setCopied] = useState(false);

  const copyCode = async () => {
    await navigator.clipboard.writeText(active.code);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  };

  return (
    <div className="overflow-hidden rounded-lg border border-dotted bg-background">
      <div className="grid lg:grid-cols-[13rem_1fr]">
        <div className="border-b border-dotted bg-muted/35 p-2 lg:border-r lg:border-b-0">
          <p className="px-3 pt-2 pb-3 font-mono text-[11px] tracking-wide text-muted-foreground uppercase">
            Provider setup
          </p>
          <div className="grid gap-1 sm:grid-cols-5 lg:grid-cols-1">
            {samples.map((sample) => (
              <button
                className="flex min-h-10 items-center justify-between rounded-md px-3 text-left text-sm text-muted-foreground transition-colors hover:bg-background hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 data-[active=true]:bg-background data-[active=true]:text-foreground data-[active=true]:shadow-sm"
                data-active={sample.id === active.id}
                key={sample.id}
                onClick={() => setActive(sample)}
                type="button"
              >
                <span>{sample.label}</span>
                {sample.id === active.id ? (
                  <Check className="size-3.5 text-[#0087a8]" />
                ) : null}
              </button>
            ))}
          </div>
        </div>
        <div>
          <div className="flex flex-col gap-3 border-b border-dotted px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="font-mono text-xs text-muted-foreground">
                storage/client.go
              </p>
              <p className="mt-1 text-sm text-foreground">{active.detail}</p>
            </div>
            <div className="flex items-center gap-2">
              <button
                className="inline-flex h-8 items-center gap-1.5 rounded-full px-3 text-sm font-medium text-[#007a99] transition-colors hover:bg-[#00add8]/10 hover:text-[#005b73] focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                onClick={copyCode}
                type="button"
              >
                {copied ? (
                  <Check className="size-3.5" />
                ) : (
                  <Copy className="size-3.5" />
                )}
                {copied ? "Copied" : "Copy"}
              </button>
              <Link
                className="hidden h-8 items-center gap-1.5 rounded-full px-3 text-sm font-medium text-[#007a99] transition-colors hover:bg-[#00add8]/10 hover:text-[#005b73] focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 sm:inline-flex"
                href={active.href}
              >
                View more
                <ArrowRight className="size-3.5" />
              </Link>
            </div>
          </div>
          <div className="max-h-[29rem] overflow-x-auto bg-[#f8fafc] dark:bg-[#0b0f14]">
            <div
              className="code-switcher-shiki code-switcher-shiki-light dark:hidden"
              dangerouslySetInnerHTML={{ __html: active.highlightedHtmlLight }}
            />
            <div
              className="code-switcher-shiki code-switcher-shiki-dark hidden dark:block"
              dangerouslySetInnerHTML={{ __html: active.highlightedHtmlDark }}
            />
          </div>
          <div className="flex flex-col gap-3 border-t border-dotted px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
            <p className="max-w-[48ch] text-sm leading-relaxed text-muted-foreground">
              The adapter changes. The upload call underneath stays the same.
            </p>
            <Link
              className="inline-flex items-center gap-1.5 text-sm font-medium text-[#007a99] transition-colors hover:text-[#005b73]"
              href="/adapters"
            >
              View more adapters
              <ArrowRight className="size-3.5" />
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
};
