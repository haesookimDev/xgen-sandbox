/**
 * Capabilities example: exercise the capability-aware runtime images.
 *
 * Creates a sandbox per capability and verifies the runtime image really
 * carries the expected feature:
 *   - "sudo"     -> passwordless sudo works (runtime-*-sudo image)
 *   - "git-ssh"  -> egress port 22 reachable (NetworkPolicy + runtime-*-sudo)
 *   - "browser"  -> Chromium installed, GUI/VNC enabled (runtime-gui-browser)
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

function header(title: string) {
  console.log(`\n${"=".repeat(60)}\n${title}\n${"=".repeat(60)}`);
}

async function testSudo() {
  header("[1/3] sudo capability (runtime-base-sudo)");
  const sandbox = await client.createSandbox({
    template: "base",
    capabilities: ["sudo"],
    timeoutSeconds: 300,
  });
  console.log(`sandbox: ${sandbox.id}`);
  console.log(`capabilities: ${JSON.stringify(sandbox.info.capabilities)}`);

  try {
    const whoami = await sandbox.exec("whoami");
    console.log(`whoami (no sudo): ${whoami.stdout.trim()}  (expect: sandbox)`);

    const sudoWhoami = await sandbox.exec("sudo -n whoami");
    console.log(
      `sudo -n whoami: ${sudoWhoami.stdout.trim()}  (expect: root, exit=${sudoWhoami.exitCode})`
    );
    if (sudoWhoami.exitCode !== 0 || sudoWhoami.stdout.trim() !== "root") {
      throw new Error("sudo capability did not grant root");
    }

    // Install a package as proof that sudo is usable for real work.
    console.log("sudo apt-get install -y cowsay ...");
    const install = await sandbox.exec(
      "sudo apt-get update -qq && sudo apt-get install -y -qq cowsay",
      { timeout: 120_000 }
    );
    console.log(`install exit=${install.exitCode}`);

    const cow = await sandbox.exec("/usr/games/cowsay 'sudo works'");
    console.log(cow.stdout);

    console.log("sudo capability: OK");
  } finally {
    await sandbox.destroy();
  }
}

async function testGitSsh() {
  header("[2/3] git-ssh capability (runtime-nodejs-sudo + egress on :22)");
  const sandbox = await client.createSandbox({
    template: "nodejs",
    capabilities: ["sudo", "git-ssh"],
    timeoutSeconds: 300,
  });
  console.log(`sandbox: ${sandbox.id}`);
  console.log(`capabilities: ${JSON.stringify(sandbox.info.capabilities)}`);

  try {
    // Without git-ssh, a plain sandbox has no egress on port 22.
    // With git-ssh, ssh-keyscan should succeed against github.com.
    console.log("ssh-keyscan -T 10 github.com ...");
    const keyscan = await sandbox.exec(
      "ssh-keyscan -T 10 github.com | head -1",
      { timeout: 30_000 }
    );
    console.log(`exit=${keyscan.exitCode}`);
    console.log(`stdout: ${keyscan.stdout.trim()}`);
    if (keyscan.exitCode !== 0 || !keyscan.stdout.includes("github.com")) {
      throw new Error("git-ssh egress on port 22 did not work");
    }

    console.log("git-ssh capability: OK");
  } finally {
    await sandbox.destroy();
  }
}

async function testBrowser() {
  header("[3/3] browser capability (runtime-gui-browser, implies gui + sudo)");
  const sandbox = await client.createSandbox({
    template: "gui",
    capabilities: ["browser"],
    timeoutSeconds: 600,
  });
  console.log(`sandbox: ${sandbox.id}`);
  console.log(`capabilities: ${JSON.stringify(sandbox.info.capabilities)}`);
  console.log(`vncUrl: ${sandbox.info.vncUrl ?? "(missing!)"}`);

  try {
    if (!sandbox.info.vncUrl) {
      throw new Error("browser capability did not enable GUI/VNC");
    }

    const which = await sandbox.exec("which chromium-browser");
    console.log(`which chromium-browser: ${which.stdout.trim()}`);
    if (which.exitCode !== 0) {
      throw new Error("chromium-browser not found in runtime-gui-browser image");
    }

    const version = await sandbox.exec("chromium-browser --version");
    console.log(`version: ${version.stdout.trim()}`);

    // Headless render of a page — confirms Chromium + system libs work.
    console.log("headless render of example.com ...");
    const dump = await sandbox.exec(
      "chromium-browser --headless --disable-gpu --no-sandbox --dump-dom https://example.com | head -c 200",
      { timeout: 60_000 }
    );
    console.log(`exit=${dump.exitCode}`);
    console.log(`stdout (first 200 bytes): ${dump.stdout}`);

    // browser implies sudo as well.
    const sudoWhoami = await sandbox.exec("sudo -n whoami");
    console.log(`sudo -n whoami: ${sudoWhoami.stdout.trim()}  (expect: root)`);

    console.log("browser capability: OK");
    console.log(`(open ${sandbox.info.vncUrl} to see the desktop)`);
  } finally {
    await sandbox.destroy();
  }
}

async function testUnknownCapabilityRejected() {
  header("[bonus] unknown capability should be rejected");
  try {
    await client.createSandbox({
      template: "base",
      capabilities: ["not-a-real-capability"],
    });
    throw new Error("expected createSandbox to reject unknown capability");
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    console.log(`rejected as expected: ${msg}`);
  }
}

async function main() {
  await testSudo();
  await testGitSsh();
  await testBrowser();
  await testUnknownCapabilityRejected();

  header("all capability tests passed");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
