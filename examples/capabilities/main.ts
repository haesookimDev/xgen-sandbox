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

    // Diagnostics for the setuid-on-exec path that sudo depends on.
    const diag = await sandbox.exec(
      "printf 'nnp='; grep -s NoNewPrivs /proc/self/status | awk '{print $2}'; " +
        "printf 'root_mount_opts='; awk '$5==\"/\"{print $6}' /proc/self/mountinfo | head -1; " +
        "printf 'sudo_perm='; stat -c '%a %U:%G' /usr/bin/sudo"
    );
    console.log(diag.stdout.trim());

    const sudoWhoami = await sandbox.exec("sudo -n whoami");
    console.log(
      `sudo -n whoami: stdout=${sudoWhoami.stdout.trim()} stderr=${sudoWhoami.stderr.trim()} exit=${sudoWhoami.exitCode}`
    );
    if (sudoWhoami.exitCode !== 0 || sudoWhoami.stdout.trim() !== "root") {
      throw new Error("sudo capability did not grant root");
    }

    // Prove sudo can actually write to a root-owned path.
    const touch = await sandbox.exec("sudo touch /root/sudo-worked && sudo ls -l /root/sudo-worked");
    console.log(`sudo file write: exit=${touch.exitCode} ${touch.stdout.trim()}`);
    if (touch.exitCode !== 0) {
      throw new Error("sudo could not write to /root");
    }

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
    // Without git-ssh, the default sandbox NetworkPolicy blocks port 22
    // egress. With git-ssh, a per-pod NetworkPolicy opens it. We test this
    // with bash's /dev/tcp — no openssh-client needed in the runtime image,
    // and github.com:22 sends an SSH banner the moment the TCP connection
    // is accepted.
    //
    // NB: kindnet (the default CNI on `kind`) does NOT enforce
    // NetworkPolicies, so on a kind cluster this test passes even without
    // the git-ssh capability. On clusters with Calico/Cilium it's a real
    // test.
    console.log("bash /dev/tcp github.com:22 ...");
    const tcp = await sandbox.exec(
      `bash -c 'exec 3<>/dev/tcp/github.com/22 && IFS= read -r -t 10 line <&3 && printf %s "$line"'`,
      { timeout: 30_000 }
    );
    console.log(`exit=${tcp.exitCode}`);
    console.log(`banner: ${tcp.stdout.trim() || "(empty)"}`);
    console.log(`stderr: ${tcp.stderr.trim() || "(empty)"}`);
    if (tcp.exitCode !== 0 || !tcp.stdout.startsWith("SSH-")) {
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
