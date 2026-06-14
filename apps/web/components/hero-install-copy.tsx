"use client";

import { Check, Copy } from "lucide-react";
import { useState } from "react";

const installCommand = "go get github.com/cersho/gofiles-sdk";

export const HeroInstallCopy = () => {
  const [copied, setCopied] = useState(false);

  const copyInstallCommand = async () => {
    try {
      await navigator.clipboard.writeText(installCommand);
    } catch {
      // Browsers can block clipboard writes outside secure/active contexts.
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  };

  return (
    <button
      aria-label={copied ? "Copied install command" : "Copy install command"}
      className="group mx-auto mt-8 inline-flex min-h-10 max-w-full items-center gap-3 rounded-lg bg-background px-3 py-2 font-mono text-xs text-foreground shadow-[var(--shadow-border)] transition-[scale,background-color,box-shadow] duration-150 ease-out hover:bg-muted/35 hover:shadow-[var(--shadow-border-hover)] focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.96]"
      onClick={copyInstallCommand}
      type="button"
    >
      <span className="text-muted-foreground">$</span>
      <code className="overflow-x-auto whitespace-nowrap">{installCommand}</code>
      <span className="relative size-3.5 shrink-0 text-[#0087a8]">
        <Copy
          aria-hidden="true"
          className={`absolute inset-0 size-3.5 transition-[scale,opacity,filter] duration-300 ease-[cubic-bezier(0.2,0,0,1)] ${
            copied
              ? "scale-[0.25] opacity-0 blur-[4px]"
              : "scale-100 opacity-100 blur-0"
          }`}
        />
        <Check
          aria-hidden="true"
          className={`absolute inset-0 size-3.5 transition-[scale,opacity,filter] duration-300 ease-[cubic-bezier(0.2,0,0,1)] ${
            copied
              ? "scale-100 opacity-100 blur-0"
              : "scale-[0.25] opacity-0 blur-[4px]"
          }`}
        />
      </span>
    </button>
  );
};
