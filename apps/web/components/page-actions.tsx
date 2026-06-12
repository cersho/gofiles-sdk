"use client";

import { Check, ChevronDown, Copy, ExternalLink, FileText } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

interface PageActionsProps {
  markdown: string;
  markdownUrl: string;
  markdownAbsoluteUrl: string;
  githubUrl: string;
}

export const PageActions = ({
  markdown,
  markdownUrl,
  markdownAbsoluteUrl,
  githubUrl,
}: PageActionsProps) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(markdown);
      setCopied(true);
      setTimeout(() => setCopied(false), 1800);
    } catch {
      // Clipboard access can be unavailable in restricted browser contexts.
    }
  };

  const chatGptUrl = `https://chatgpt.com/?${new URLSearchParams({
    hints: "search",
    prompt: `Read ${markdownAbsoluteUrl}. I want to ask questions about it.`,
  })}`;

  return (
    <div className="not-prose mb-6 flex flex-row items-center gap-2">
      <Button onClick={handleCopy} size="sm" type="button" variant="outline">
        {copied ? (
          <Check data-icon="inline-start" />
        ) : (
          <Copy data-icon="inline-start" />
        )}
        {copied ? "Copied" : "Copy Markdown"}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="outline">
            Open
            <ChevronDown data-icon="inline-end" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-52">
          <DropdownMenuGroup>
            <DropdownMenuItem asChild>
              <a href={markdownUrl} rel="noopener noreferrer" target="_blank">
                <FileText />
                View as Markdown
              </a>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <a href={githubUrl} rel="noopener noreferrer" target="_blank">
                <ExternalLink />
                Open in GitHub
              </a>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <a href={chatGptUrl} rel="noopener noreferrer" target="_blank">
                <FileText />
                Ask with Markdown
              </a>
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
};
