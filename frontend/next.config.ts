import type { NextConfig } from "next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants";

// The production export is served by Go on the same origin.
export default function config(phase: string): NextConfig {
  if (phase === PHASE_DEVELOPMENT_SERVER)
    return {
      devIndicators: false,
      async rewrites() {
        return [{ source: "/api/:path*", destination: "http://127.0.0.1:8080/api/:path*" }];
      },
    };
  return { output: "export", trailingSlash: true };
}
