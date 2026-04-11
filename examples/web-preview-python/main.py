"""
Web preview example: Deploy a simple web server in a sandbox and get its preview URL.

Usage:
  pip install -r requirements.txt
  python main.py

Prerequisites:
  - xgen-sandbox agent running (make dev-deploy)
  - AGENT_URL and API_KEY environment variables set
"""

import asyncio
import os
import signal

from xgen_sandbox import XgenClient

SERVER_CODE = """
const http = require('http');

const server = http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html' });
  res.end('<h1>Hello from xgen-sandbox!</h1><p>This is running inside a Kubernetes pod.</p>');
});

server.listen(3000, '0.0.0.0', () => {
  console.log('Server running on port 3000');
});
"""


async def main() -> None:
    api_key = os.environ.get("API_KEY", "xgen-local-api-key-2026")
    agent_url = os.environ.get("AGENT_URL", "http://localhost:8080")

    async with XgenClient(api_key=api_key, agent_url=agent_url) as client:
        print("Creating Node.js sandbox with port 3000...")
        sandbox = await client.create_sandbox(
            template="nodejs",
            ports=[3000],
            timeout_seconds=600,
        )
        print(f"Sandbox: {sandbox.id}")
        print(f"Preview URL: {sandbox.get_preview_url(3000)}")

        try:
            # Write the server file
            await sandbox.write_file("server.js", SERVER_CODE)

            # Listen for port open events
            port_watcher = sandbox.on_port_open(
                lambda port: print(f"Port {port} is now open! Visit: {sandbox.get_preview_url(port)}")
            )

            # Start the server
            print("\nStarting web server...")
            result = await sandbox.exec("node server.js &")
            print(f"Server started (exit code: {result.exit_code})")

            # Verify the server is running
            await asyncio.sleep(2)
            check = await sandbox.exec("curl -s http://localhost:3000")
            print(f"Response: {check.stdout[:80]}...")

            print(f"\nServer is ready! Open: {sandbox.get_preview_url(3000)}")
            print("Press Ctrl+C to stop.")

            # Wait until interrupted
            stop = asyncio.Event()
            loop = asyncio.get_event_loop()
            loop.add_signal_handler(signal.SIGINT, stop.set)
            await stop.wait()

        finally:
            print("\nCleaning up...")
            await sandbox.destroy()


if __name__ == "__main__":
    asyncio.run(main())
