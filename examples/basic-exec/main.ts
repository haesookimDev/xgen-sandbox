/**
 * Basic example: Create a sandbox and execute a command.
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
  console.log("Creating sandbox...");
  const sandbox = await client.createSandbox({
    template: "base",
    timeoutSeconds: 300,
  });
  console.log(`Sandbox created: ${sandbox.id} (status: ${sandbox.status})`);

  try {
    // Execute a simple command
    console.log("\nRunning: echo 'Hello from xgen-sandbox!'");
    const result = await sandbox.exec("echo 'Hello from xgen-sandbox!'");
    console.log(`Exit code: ${result.exitCode}`);
    console.log(`Stdout: ${result.stdout}`);

    // Execute a multi-step command
    console.log("\nRunning: uname -a");
    const sysInfo = await sandbox.exec("uname -a");
    console.log(`System: ${sysInfo.stdout}`);

    // Write and read a file
    console.log("\nWriting file...");
    await sandbox.writeFile("hello.txt", "Hello, World!\n");
    const content = await sandbox.readTextFile("hello.txt");
    console.log(`File content: ${content}`);

    // List directory
    console.log("\nListing workspace:");
    const files = await sandbox.listDir(".");
    for (const file of files) {
      console.log(`  ${file.isDir ? "d" : "-"} ${file.name} (${file.size} bytes)`);
    }

    // Stream exec output
    console.log("\nStreaming: for i in 1 2 3; do echo $i; sleep 0.5; done");
    for await (const event of sandbox.execStream(
      "bash -c 'for i in 1 2 3; do echo $i; sleep 0.5; done'"
    )) {
      if (event.type === "stdout") {
        process.stdout.write(`  [stdout] ${event.data}`);
      } else if (event.type === "exit") {
        console.log(`  [exit] code=${event.exitCode}`);
      }
    }
  } finally {
    console.log("\nDestroying sandbox...");
    await sandbox.destroy();
    console.log("Done.");
  }
}

main().catch(console.error);
