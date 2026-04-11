// Basic example: Create a sandbox and execute a command.
//
// Usage:
//
//	go run main.go
//
// Prerequisites:
//   - xgen-sandbox agent running (make dev-deploy)
//   - AGENT_URL and API_KEY environment variables set
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	xgen "github.com/xgen-sandbox/sdk-go"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	apiKey := getEnv("API_KEY", "xgen-local-api-key-2026")
	agentURL := getEnv("AGENT_URL", "http://localhost:8080")

	ctx := context.Background()
	client := xgen.NewClient(apiKey, agentURL)

	fmt.Println("Creating sandbox...")
	sandbox, err := client.CreateSandbox(ctx, xgen.CreateSandboxOptions{
		Template:       "base",
		TimeoutSeconds: 300,
	})
	if err != nil {
		log.Fatalf("Failed to create sandbox: %v", err)
	}
	fmt.Printf("Sandbox created: %s (status: %s)\n", sandbox.ID, sandbox.Status())
	defer func() {
		fmt.Println("\nDestroying sandbox...")
		if err := sandbox.Destroy(ctx); err != nil {
			log.Printf("Failed to destroy sandbox: %v", err)
		}
		fmt.Println("Done.")
	}()

	// Execute a simple command
	fmt.Println("\nRunning: echo 'Hello from xgen-sandbox!'")
	result, err := sandbox.Exec(ctx, "echo 'Hello from xgen-sandbox!'")
	if err != nil {
		log.Fatalf("Exec failed: %v", err)
	}
	fmt.Printf("Exit code: %d\n", result.ExitCode)
	fmt.Printf("Stdout: %s", result.Stdout)

	// Execute a multi-step command
	fmt.Println("\nRunning: uname -a")
	sysInfo, err := sandbox.Exec(ctx, "uname -a")
	if err != nil {
		log.Fatalf("Exec failed: %v", err)
	}
	fmt.Printf("System: %s", sysInfo.Stdout)

	// Write and read a file
	fmt.Println("\nWriting file...")
	if err := sandbox.WriteFile(ctx, "hello.txt", []byte("Hello, World!\n")); err != nil {
		log.Fatalf("WriteFile failed: %v", err)
	}
	content, err := sandbox.ReadTextFile(ctx, "hello.txt")
	if err != nil {
		log.Fatalf("ReadTextFile failed: %v", err)
	}
	fmt.Printf("File content: %s", content)

	// List directory
	fmt.Println("\nListing workspace:")
	files, err := sandbox.ListDir(ctx, ".")
	if err != nil {
		log.Fatalf("ListDir failed: %v", err)
	}
	for _, f := range files {
		kind := "-"
		if f.IsDir {
			kind = "d"
		}
		fmt.Printf("  %s %s (%d bytes)\n", kind, f.Name, f.Size)
	}
}
