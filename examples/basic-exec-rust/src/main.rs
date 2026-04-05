//! Basic example: Create a sandbox and execute a command.
//!
//! Usage:
//!   cargo run
//!
//! Prerequisites:
//!   - xgen-sandbox agent running (make dev-deploy)
//!   - AGENT_URL and API_KEY environment variables set

use xgen_sandbox::{CreateSandboxOptions, XgenClient};

fn env_or(key: &str, fallback: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| fallback.to_string())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let api_key = env_or("API_KEY", "xgen_dev_key");
    let agent_url = env_or("AGENT_URL", "http://localhost:8080");

    let client = XgenClient::new(&api_key, &agent_url);

    println!("Creating sandbox...");
    let sandbox = client
        .create_sandbox(CreateSandboxOptions {
            template: Some("base".to_string()),
            timeout_seconds: Some(300),
            ..Default::default()
        })
        .await?;
    println!("Sandbox created: {} (status: {:?})", sandbox.id, sandbox.status().await);

    // Execute a simple command
    println!("\nRunning: echo 'Hello from xgen-sandbox!'");
    let result = sandbox.exec("echo 'Hello from xgen-sandbox!'", None).await?;
    println!("Exit code: {}", result.exit_code);
    println!("Stdout: {}", result.stdout);

    // Execute a multi-step command
    println!("\nRunning: uname -a");
    let sys_info = sandbox.exec("uname -a", None).await?;
    println!("System: {}", sys_info.stdout);

    // Write and read a file
    println!("\nWriting file...");
    sandbox.write_file("hello.txt", b"Hello, World!\n").await?;
    let content = sandbox.read_text_file("hello.txt").await?;
    println!("File content: {content}");

    // List directory
    println!("\nListing workspace:");
    let files = sandbox.list_dir(".").await?;
    for f in &files {
        let kind = if f.is_dir { "d" } else { "-" };
        println!("  {kind} {} ({} bytes)", f.name, f.size);
    }

    println!("\nDestroying sandbox...");
    sandbox.destroy().await?;
    println!("Done.");

    Ok(())
}
