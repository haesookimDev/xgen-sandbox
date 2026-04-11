/**
 * Web preview example: Deploy a simple web server in a sandbox and get its preview URL.
 *
 * Usage:
 *   npx tsx main.ts
 */

import { XgenClient } from "@xgen-sandbox/sdk";

const client = new XgenClient({
  apiKey: process.env.API_KEY ?? "xgen-local-api-key-2026",
  agentUrl: process.env.AGENT_URL ?? "http://localhost:8080",
});

async function main() {
  console.log("Creating Node.js sandbox with port 3000...");
  const sandbox = await client.createSandbox({
    template: "nodejs",
    ports: [3000],
    timeoutSeconds: 600,
  });
  console.log(`Sandbox: ${sandbox.id}`);
  console.log(`Preview URL: ${sandbox.getPreviewUrl(3000)}`);

  try {
    // Write a simple Express server
    await sandbox.writeFile(
      "server.js",
      `
const http = require('http');

const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html' });
  res.end('<h1>Hello from xgen-sandbox!</h1><p>This is running inside a Kubernetes pod.</p>');
});

server.listen(3000, '0.0.0.0', () => {
  console.log('Server running on port 3000');
});
`
    );

    // Start the server (non-blocking)
    console.log("\nStarting web server...");
    const execIter = sandbox.execStream("node server.js");

    // Wait for the server to start by watching for port open
    sandbox.onPortOpen((port) => {
      console.log(`Port ${port} is now open!`);
      console.log(`Visit: ${sandbox.getPreviewUrl(port)}`);
    });

    // Stream output for a bit
    console.log("Server output:");
    for await (const event of execIter) {
      if (event.type === "stdout") {
        console.log(`  ${event.data}`);
        // Once we see "Server running", we know it's ready
        if (event.data?.includes("Server running")) {
          console.log(`\nServer is ready! Open: ${sandbox.getPreviewUrl(3000)}`);
          console.log("Press Ctrl+C to stop.");
          break;
        }
      }
    }

    // Keep alive until user cancels
    await new Promise((resolve) => {
      process.on("SIGINT", resolve);
    });
  } finally {
    console.log("\nCleaning up...");
    await sandbox.destroy();
  }
}

main().catch(console.error);
