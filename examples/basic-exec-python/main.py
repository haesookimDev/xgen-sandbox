"""
Basic example: Create a sandbox and execute a command.

Usage:
  pip install -r requirements.txt
  python main.py

Prerequisites:
  - xgen-sandbox agent running (make dev-deploy)
  - AGENT_URL and API_KEY environment variables set
"""

import asyncio
import os

from xgen_sandbox import XgenClient


async def main() -> None:
    api_key = os.environ.get("API_KEY", "xgen-local-api-key-2026")
    agent_url = os.environ.get("AGENT_URL", "http://localhost:8080")

    async with XgenClient(api_key=api_key, agent_url=agent_url) as client:
        print("Creating sandbox...")
        sandbox = await client.create_sandbox(template="base", timeout_seconds=300)
        print(f"Sandbox created: {sandbox.id} (status: {sandbox.status})")

        try:
            # Execute a simple command
            print("\nRunning: echo 'Hello from xgen-sandbox!'")
            result = await sandbox.exec("echo 'Hello from xgen-sandbox!'")
            print(f"Exit code: {result.exit_code}")
            print(f"Stdout: {result.stdout}")

            # Execute a multi-step command
            print("\nRunning: uname -a")
            sys_info = await sandbox.exec("uname -a")
            print(f"System: {sys_info.stdout}")

            # Write and read a file
            print("\nWriting file...")
            await sandbox.write_file("hello.txt", "Hello, World!\n")
            content = await sandbox.read_text_file("hello.txt")
            print(f"File content: {content}")

            # List directory
            print("\nListing workspace:")
            files = await sandbox.list_dir(".")
            for f in files:
                kind = "d" if f.is_dir else "-"
                print(f"  {kind} {f.name} ({f.size} bytes)")

        finally:
            print("\nDestroying sandbox...")
            await sandbox.destroy()
            print("Done.")


if __name__ == "__main__":
    asyncio.run(main())
