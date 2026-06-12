import { ArrowLeft, ArrowRight } from "lucide-react";
import type { Metadata } from "next";
import Link from "next/link";

import { Button } from "@/components/ui/button";

export const metadata: Metadata = {
  title: "Page not found",
};

const NotFound = () => (
  <main className="fixed inset-0 z-50 flex flex-col items-center justify-center overflow-y-auto bg-background px-6 py-24 text-center">
    <p className="font-mono text-xs text-muted-foreground">404</p>
    <h1 className="mt-3 max-w-[24ch] text-balance text-4xl font-medium tracking-tight text-foreground sm:text-5xl">
      This page could not be found.
    </h1>
    <p className="mt-5 max-w-[48ch] text-pretty text-base leading-relaxed text-muted-foreground sm:text-lg">
      Check the URL, or open the docs index to find the page you need.
    </p>
    <div className="mt-10 flex flex-wrap items-center justify-center gap-3">
      <Button asChild size="lg">
        <Link href="/overview">
          <ArrowLeft data-icon="inline-start" />
          Back to overview
        </Link>
      </Button>
      <Button asChild size="lg" variant="ghost">
        <Link href="/api">
          Read the API docs
          <ArrowRight data-icon="inline-end" />
        </Link>
      </Button>
    </div>
  </main>
);

export default NotFound;
