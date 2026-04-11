/**
 * GUI desktop example: Create a sandbox with VNC access for graphical applications.
 *
 * Usage:
 *   npx tsx main.ts
 *
 * Prerequisites:
 *   - xgen-sandbox agent running (make dev-deploy)
 *   - AGENT_URL and API_KEY environment variables set
 */

import { XgenClient } from "@xgen-sandbox/sdk";

const client = new XgenClient({
  apiKey: process.env.API_KEY ?? "xgen-local-api-key-2026",
  agentUrl: process.env.AGENT_URL ?? "http://localhost:8080",
});

async function main() {
  console.log("Creating GUI sandbox with VNC...");
  const sandbox = await client.createSandbox({
    template: "gui",
    gui: true,
    timeoutSeconds: 600,
  });
  console.log(`Sandbox: ${sandbox.id}`);
  console.log(`VNC URL: ${sandbox.info.vncUrl}`);
  console.log(`WS URL: ${sandbox.info.wsUrl}`);

  try {
    // Wait for desktop environment to initialize
    console.log("\nWaiting for desktop to start...");
    await new Promise((resolve) => setTimeout(resolve, 3000));

    // Launch a graphical application
    console.log("Launching xterm...");
    const result = await sandbox.exec("DISPLAY=:0 xterm &");
    console.log(`xterm launched (exit code: ${result.exitCode})`);

    // Verify the display is running
    const displayCheck = await sandbox.exec("DISPLAY=:0 xdpyinfo | head -5");
    console.log(`Display info:\n${displayCheck.stdout}`);

    console.log(`\nDesktop is ready!`);
    console.log(`Open VNC in browser: ${sandbox.info.vncUrl}`);
    console.log("Press Ctrl+C to stop.");

    // Keep sandbox alive
    const keepAliveInterval = setInterval(async () => {
      try {
        await sandbox.keepAlive();
      } catch {
        // Ignore keep-alive errors
      }
    }, 60_000);

    // Wait until user cancels
    await new Promise<void>((resolve) => {
      process.on("SIGINT", () => {
        clearInterval(keepAliveInterval);
        resolve();
      });
    });
  } finally {
    console.log("\nCleaning up...");
    await sandbox.destroy();
    console.log("Done.");
  }
}

main().catch(console.error);
