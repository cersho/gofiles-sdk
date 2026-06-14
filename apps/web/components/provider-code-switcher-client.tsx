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
    <div className="w-full min-w-0 overflow-hidden rounded-lg bg-background shadow-[var(--shadow-border)]">
      <div className="grid min-w-0 lg:grid-cols-[13rem_minmax(0,1fr)]">
        <div className="min-w-0 border-b border-dotted bg-muted/35 p-2 lg:border-r lg:border-b-0">
          <p className="px-3 pt-2 pb-3 font-mono text-[11px] tracking-wide text-muted-foreground uppercase">
            Provider setup
          </p>
          <div className="flex min-w-0 gap-1 overflow-x-auto pb-1 sm:grid sm:grid-cols-5 sm:overflow-visible sm:pb-0 lg:grid-cols-1">
            {samples.map((sample) => (
              <button
                className="flex min-h-10 min-w-max items-center justify-between gap-4 rounded-md px-3 text-left text-sm text-muted-foreground transition-[scale,background-color,color,box-shadow] duration-150 ease-out hover:bg-background hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.96] data-[active=true]:bg-background data-[active=true]:text-foreground data-[active=true]:shadow-[var(--shadow-border)] sm:min-w-0"
                data-active={sample.id === active.id}
                key={sample.id}
                onClick={() => setActive(sample)}
                type="button"
              >
                <span className="truncate">{sample.label}</span>
                <Check
                  className={`size-3.5 text-[#0087a8] transition-[scale,opacity,filter] duration-300 ease-[cubic-bezier(0.2,0,0,1)] ${
                    sample.id === active.id
                      ? "scale-100 opacity-100 blur-0"
                      : "scale-[0.25] opacity-0 blur-[4px]"
                  }`}
                />
              </button>
            ))}
          </div>
        </div>
        <div className="min-w-0">
          <div className="flex flex-col gap-3 border-b border-dotted px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <p className="font-mono text-xs text-muted-foreground">
                storage/client.go
              </p>
              <p className="mt-1 text-pretty text-sm text-foreground">
                {active.detail}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <button
                className="inline-flex h-10 items-center gap-1.5 rounded-full px-3 text-sm font-medium text-[#007a99] transition-[scale,background-color,color] duration-150 ease-out hover:bg-[#00add8]/10 hover:text-[#005b73] focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.96]"
                onClick={copyCode}
                type="button"
              >
                <span className="relative size-3.5">
                  <Copy
                    className={`absolute inset-0 size-3.5 transition-[scale,opacity,filter] duration-300 ease-[cubic-bezier(0.2,0,0,1)] ${
                      copied
                        ? "scale-[0.25] opacity-0 blur-[4px]"
                        : "scale-100 opacity-100 blur-0"
                    }`}
                  />
                  <Check
                    className={`absolute inset-0 size-3.5 transition-[scale,opacity,filter] duration-300 ease-[cubic-bezier(0.2,0,0,1)] ${
                      copied
                        ? "scale-100 opacity-100 blur-0"
                        : "scale-[0.25] opacity-0 blur-[4px]"
                    }`}
                  />
                </span>
                {copied ? "Copied" : "Copy"}
              </button>
              <Link
                className="hidden h-10 items-center gap-1.5 rounded-full px-3 text-sm font-medium text-[#007a99] transition-[scale,background-color,color] duration-150 ease-out hover:bg-[#00add8]/10 hover:text-[#005b73] focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.96] sm:inline-flex"
                href={active.href}
              >
                View more
                <ArrowRight className="size-3.5" />
              </Link>
            </div>
          </div>
          <div className="max-h-[29rem] min-w-0 overflow-x-auto bg-[#f8fafc] dark:bg-[#0b0f14]">
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
              className="inline-flex min-h-10 items-center gap-1.5 text-sm font-medium text-[#007a99] transition-colors hover:text-[#005b73]"
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
