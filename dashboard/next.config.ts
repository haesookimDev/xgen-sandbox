import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  env: {
    NEXT_PUBLIC_AGENT_URL:
      process.env.NEXT_PUBLIC_AGENT_URL || "http://localhost:8080",
  },
};

export default nextConfig;
