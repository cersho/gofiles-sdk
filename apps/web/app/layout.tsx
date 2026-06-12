import { RootProvider } from "fumadocs-ui/provider/next";
import type { Metadata } from "next";
import localFont from "next/font/local";
import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

import "./globals.css";

const geistSans = localFont({
  src: "../lib/fonts/Geist-Regular.woff",
  variable: "--font-sans",
});

const geistMono = localFont({
  src: "../lib/fonts/GeistMono-Regular.woff",
  variable: "--font-mono",
});

const protocol = process.env.NODE_ENV === "production" ? "https" : "http";
const origin = process.env.VERCEL_PROJECT_PRODUCTION_URL ?? "localhost:3000";
const baseUrl = `${protocol}://${origin}`;

const title = "Go Files SDK - one Go API for object storage";
const description =
  "Use one Go client for S3, Cloudflare R2, local files, memory storage, S3-compatible buckets, and UploadThing.";

export const metadata: Metadata = {
  alternates: {
    canonical: "/",
  },
  description,
  metadataBase: new URL(baseUrl),
  openGraph: {
    description,
    locale: "en_US",
    siteName: "Go Files SDK",
    title,
    type: "website",
    url: "/",
  },
  title: {
    default: title,
    template: "%s · Go Files SDK",
  },
  twitter: {
    card: "summary_large_image",
    description,
    title,
  },
};

const jsonLd = {
  "@context": "https://schema.org",
  "@type": "SoftwareSourceCode",
  codeRepository: "https://github.com/cersho/gofiles-sdk",
  description,
  license: "https://opensource.org/licenses/MIT",
  name: "Go Files SDK",
  programmingLanguage: "Go",
  url: baseUrl,
};

const RootLayout = ({ children }: { children: ReactNode }) => (
  <html
    lang="en"
    data-scroll-behavior="smooth"
    className={cn(
      "scroll-smooth touch-manipulation font-sans antialiased",
      geistSans.variable,
      geistMono.variable
    )}
    suppressHydrationWarning
  >
    <body className="flex min-h-full flex-col">
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
      />
      <RootProvider search={{ options: { api: "/search" } }}>
        {children}
      </RootProvider>
    </body>
  </html>
);

export default RootLayout;
