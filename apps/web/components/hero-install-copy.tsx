"use client";

import { Check, Copy } from "lucide-react";
import { useState } from "react";

const installCommand = "go get github.com/cersho/gofiles-sdk";

export const HeroInstallCopy = () => {
  const [copied, setCopied] = useState(false);

  const copyInstallCommand = async () => {
    await navigator.clipboard.writeText(installCommand);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  };

  return (
    <button
      aria-label={copied ? "Copied install command" : "Copy install command"}
      className="group mx-auto mt-8 inline-flex max-w-full items-center gap-3 rounded-md border border-dotted bg-background px-3 py-2 font-mono text-xs text-foreground transition-colors hover:border-[#00add8]/45 hover:bg-muted/35 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 active:scale-[0.99]"
      onClick={copyInstallCommand}
      type="button"
    >
      <span className="text-muted-foreground">$</span>
      <code className="overflow-x-auto whitespace-nowrap">{installCommand}</code>
      <span className="relative size-3.5 shrink-0 text-[#0087a8]">
        <Copy
          aria-hidden="true"
          className={`absolute inset-0 size-3.5 transition-[opacity,transform] duration-200 ease-out ${
            copied
              ? "scale-75 rotate-45 opacity-0"
              : "scale-100 rotate-0 opacity-100"
          }`}
        />
        <Check
          aria-hidden="true"
          className={`absolute inset-0 size-3.5 transition-[opacity,transform] duration-200 ease-out ${
            copied
              ? "scale-100 rotate-0 opacity-100"
              : "scale-75 -rotate-45 opacity-0"
          }`}
        />
      </span>
    </button>
  );
};
