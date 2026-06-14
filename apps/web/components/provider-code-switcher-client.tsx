"use client";

import { Check, Copy } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

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
  const [activeId, setActiveId] = useState(samples[0]?.id ?? "");
  const [copied, setCopied] = useState(false);

  const active = useMemo(
    () => samples.find((sample) => sample.id === activeId) ?? samples[0],
    [activeId, samples]
  );

  if (!active) {
    return null;
  }

  const copyCode = async () => {
    try {
      await navigator.clipboard.writeText(active.code);
    } catch {
      // Browsers can block clipboard writes outside secure/active contexts.
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  };

  return (
    <div className="w-full min-w-0 overflow-hidden rounded-xl bg-background shadow-[var(--shadow-border)]">
      <div className="border-b border-dotted px-3 pt-3">
        <div className="flex min-w-0 gap-5 overflow-x-auto">
          {samples.map((sample) => (
            <button
              aria-pressed={sample.id === active.id}
              className="relative inline-flex min-h-10 min-w-max items-center border-[#00add8] border-b-2 border-transparent px-0 pb-3 text-sm text-muted-foreground transition-[scale,border-color,color] duration-150 ease-out hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.96] data-[active=true]:border-[#00add8] data-[active=true]:text-foreground"
              data-active={sample.id === active.id}
              key={sample.id}
              onClick={() => setActiveId(sample.id)}
              type="button"
            >
              <span>{sample.label}</span>
            </button>
          ))}
          <Link
            className="inline-flex min-h-10 min-w-max items-center border-transparent border-b-2 px-0 pb-3 text-sm font-medium text-[#007a99] transition-[scale,color] duration-150 ease-out hover:text-[#005b73] focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.96]"
            href="/adapters"
          >
            See More
          </Link>
        </div>
      </div>

      <div className="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 text-left">
          <p className="font-mono text-[11px] tracking-wide text-muted-foreground uppercase">
            storage/client.go
          </p>
          <p className="mt-2 max-w-[56ch] text-pretty text-sm text-foreground">
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
            className="hidden h-10 items-center rounded-full px-3 text-sm font-medium text-muted-foreground transition-[scale,color] duration-150 ease-out hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.96] sm:inline-flex"
            href={active.href}
          >
            Adapter docs
          </Link>
        </div>
      </div>

      <div className="max-h-[29rem] min-w-0 overflow-x-auto border-t border-dotted bg-[#f8fafc] dark:bg-[#0b0f14]">
        <div
          className="code-switcher-shiki code-switcher-shiki-light dark:hidden"
          dangerouslySetInnerHTML={{ __html: active.highlightedHtmlLight }}
        />
        <div
          className="code-switcher-shiki code-switcher-shiki-dark hidden dark:block"
          dangerouslySetInnerHTML={{ __html: active.highlightedHtmlDark }}
        />
      </div>

      <div className="flex items-center justify-between gap-3 px-4 py-3 text-sm text-muted-foreground">
        <span>Provider setup moves. Calls stay stable.</span>
        <Link className="font-medium text-[#007a99] hover:text-[#005b73]" href="/adapters">
          All adapters
        </Link>
      </div>
    </div>
  );
};
