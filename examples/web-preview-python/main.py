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

            # Start the server (non-blocking via exec_stream)
            print("\nStarting web server...")
            print("Server output:")
            async for event in sandbox.exec_stream("node server.js"):
                if event.type == "stdout":
                    print(f"  {event.data}", end="")
                    if "Server running" in (event.data or ""):
                        print(f"\nServer is ready! Open: {sandbox.get_preview_url(3000)}")
                        print("Press Ctrl+C to stop.")
                        break

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
