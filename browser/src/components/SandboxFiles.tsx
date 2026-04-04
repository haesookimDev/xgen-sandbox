import { useCallback, useEffect, useState } from "react";
import type { FileEntry, SandboxFilesProps } from "../types.js";

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(unix: number): string {
  return new Date(unix * 1000).toLocaleString();
}

export function SandboxFiles({
  listDir,
  readFile,
  writeFile,
  deleteFile,
  initialPath = ".",
  className,
  style,
  onFileSelect,
}: SandboxFilesProps) {
  const [currentPath, setCurrentPath] = useState(initialPath);
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<string | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [editContent, setEditContent] = useState("");

  const loadDirectory = useCallback(
    async (path: string) => {
      setLoading(true);
      setError(null);
      setSelectedFile(null);
      setFileContent(null);
      setEditMode(false);
      try {
        const items = await listDir(path);
        // Sort: directories first, then alphabetically
        items.sort((a, b) => {
          if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
          return a.name.localeCompare(b.name);
        });
        setEntries(items);
        setCurrentPath(path);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load directory");
      } finally {
        setLoading(false);
      }
    },
    [listDir]
  );

  useEffect(() => {
    loadDirectory(initialPath);
  }, [initialPath, loadDirectory]);

  const handleEntryClick = useCallback(
    async (entry: FileEntry) => {
      const entryPath =
        currentPath === "." ? entry.name : `${currentPath}/${entry.name}`;
      if (entry.isDir) {
        loadDirectory(entryPath);
      } else {
        setSelectedFile(entryPath);
        try {
          const content = await readFile(entryPath);
          setFileContent(content);
          onFileSelect?.(entryPath, content);
        } catch {
          setFileContent("(Failed to read file)");
        }
      }
    },
    [currentPath, loadDirectory, readFile, onFileSelect]
  );

  const handleNavigateUp = useCallback(() => {
    if (currentPath === "." || currentPath === "/") return;
    const parts = currentPath.split("/");
    parts.pop();
    const parent = parts.length === 0 ? "." : parts.join("/");
    loadDirectory(parent);
  }, [currentPath, loadDirectory]);

  const handleSave = useCallback(async () => {
    if (!writeFile || !selectedFile) return;
    try {
      await writeFile(selectedFile, editContent);
      setFileContent(editContent);
      setEditMode(false);
    } catch {
      setError("Failed to save file");
    }
  }, [writeFile, selectedFile, editContent]);

  const handleDelete = useCallback(
    async (entry: FileEntry) => {
      if (!deleteFile) return;
      const entryPath =
        currentPath === "." ? entry.name : `${currentPath}/${entry.name}`;
      try {
        await deleteFile(entryPath);
        loadDirectory(currentPath);
      } catch {
        setError("Failed to delete file");
      }
    },
    [deleteFile, currentPath, loadDirectory]
  );

  const breadcrumbs = currentPath === "." ? ["workspace"] : ["workspace", ...currentPath.split("/")];

  return (
    <div
      className={className}
      style={{
        display: "flex",
        flexDirection: "column",
        width: "100%",
        height: "100%",
        border: "1px solid #e2e8f0",
        borderRadius: "8px",
        overflow: "hidden",
        backgroundColor: "#fff",
        fontSize: "13px",
        ...style,
      }}
    >
      {/* Breadcrumb bar */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "4px",
          padding: "8px 12px",
          backgroundColor: "#f8fafc",
          borderBottom: "1px solid #e2e8f0",
          fontFamily: "monospace",
          flexWrap: "wrap",
        }}
      >
        {breadcrumbs.map((part, i) => (
          <span key={i}>
            {i > 0 && <span style={{ color: "#94a3b8", margin: "0 2px" }}>/</span>}
            <span
              onClick={() => {
                if (i === 0) loadDirectory(".");
                else {
                  const target = currentPath
                    .split("/")
                    .slice(0, i)
                    .join("/");
                  loadDirectory(target || ".");
                }
              }}
              style={{
                cursor: "pointer",
                color: i === breadcrumbs.length - 1 ? "#1e293b" : "#3b82f6",
              }}
            >
              {part}
            </span>
          </span>
        ))}
      </div>

      <div style={{ display: "flex", flex: 1, overflow: "hidden" }}>
        {/* File list */}
        <div
          style={{
            width: selectedFile ? "40%" : "100%",
            borderRight: selectedFile ? "1px solid #e2e8f0" : "none",
            overflow: "auto",
          }}
        >
          {error && (
            <div style={{ padding: "12px", color: "#ef4444" }}>{error}</div>
          )}

          {loading ? (
            <div style={{ padding: "12px", color: "#64748b" }}>Loading...</div>
          ) : (
            <table style={{ width: "100%", borderCollapse: "collapse" }}>
              <tbody>
                {currentPath !== "." && currentPath !== "/" && (
                  <tr
                    onClick={handleNavigateUp}
                    style={{ cursor: "pointer" }}
                    onMouseEnter={(e) =>
                      (e.currentTarget.style.backgroundColor = "#f1f5f9")
                    }
                    onMouseLeave={(e) =>
                      (e.currentTarget.style.backgroundColor = "")
                    }
                  >
                    <td style={{ padding: "6px 12px" }}>..</td>
                    <td />
                    <td />
                    <td />
                  </tr>
                )}
                {entries.map((entry) => (
                  <tr
                    key={entry.name}
                    onClick={() => handleEntryClick(entry)}
                    style={{
                      cursor: "pointer",
                      backgroundColor:
                        selectedFile &&
                        selectedFile.endsWith("/" + entry.name)
                          ? "#eff6ff"
                          : undefined,
                    }}
                    onMouseEnter={(e) => {
                      if (!e.currentTarget.style.backgroundColor?.includes("eff6ff"))
                        e.currentTarget.style.backgroundColor = "#f1f5f9";
                    }}
                    onMouseLeave={(e) => {
                      if (!e.currentTarget.style.backgroundColor?.includes("eff6ff"))
                        e.currentTarget.style.backgroundColor = "";
                    }}
                  >
                    <td
                      style={{
                        padding: "6px 12px",
                        fontFamily: "monospace",
                        color: entry.isDir ? "#3b82f6" : "#1e293b",
                        fontWeight: entry.isDir ? 600 : 400,
                      }}
                    >
                      {entry.isDir ? "\u{1F4C1}" : "\u{1F4C4}"} {entry.name}
                      {entry.isDir ? "/" : ""}
                    </td>
                    <td
                      style={{
                        padding: "6px 8px",
                        color: "#64748b",
                        textAlign: "right",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {entry.isDir ? "-" : formatSize(entry.size)}
                    </td>
                    <td
                      style={{
                        padding: "6px 8px",
                        color: "#94a3b8",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {formatDate(entry.modTime)}
                    </td>
                    <td style={{ padding: "6px 8px", textAlign: "right" }}>
                      {deleteFile && !entry.isDir && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDelete(entry);
                          }}
                          style={{
                            border: "none",
                            background: "none",
                            cursor: "pointer",
                            color: "#ef4444",
                            fontSize: "12px",
                            padding: "2px 6px",
                            borderRadius: "4px",
                          }}
                          title="Delete"
                        >
                          &#x2715;
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
                {entries.length === 0 && !loading && (
                  <tr>
                    <td
                      colSpan={4}
                      style={{
                        padding: "24px 12px",
                        color: "#94a3b8",
                        textAlign: "center",
                      }}
                    >
                      Empty directory
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          )}
        </div>

        {/* File content panel */}
        {selectedFile && (
          <div
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
            }}
          >
            <div
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                padding: "6px 12px",
                backgroundColor: "#f8fafc",
                borderBottom: "1px solid #e2e8f0",
              }}
            >
              <span
                style={{
                  fontFamily: "monospace",
                  fontSize: "12px",
                  color: "#64748b",
                }}
              >
                {selectedFile}
              </span>
              <div style={{ display: "flex", gap: "4px" }}>
                {writeFile && !editMode && (
                  <button
                    onClick={() => {
                      setEditContent(fileContent ?? "");
                      setEditMode(true);
                    }}
                    style={{
                      border: "1px solid #e2e8f0",
                      background: "#fff",
                      cursor: "pointer",
                      padding: "2px 8px",
                      borderRadius: "4px",
                      fontSize: "12px",
                    }}
                  >
                    Edit
                  </button>
                )}
                {editMode && (
                  <>
                    <button
                      onClick={handleSave}
                      style={{
                        border: "1px solid #3b82f6",
                        background: "#3b82f6",
                        color: "#fff",
                        cursor: "pointer",
                        padding: "2px 8px",
                        borderRadius: "4px",
                        fontSize: "12px",
                      }}
                    >
                      Save
                    </button>
                    <button
                      onClick={() => setEditMode(false)}
                      style={{
                        border: "1px solid #e2e8f0",
                        background: "#fff",
                        cursor: "pointer",
                        padding: "2px 8px",
                        borderRadius: "4px",
                        fontSize: "12px",
                      }}
                    >
                      Cancel
                    </button>
                  </>
                )}
                <button
                  onClick={() => {
                    setSelectedFile(null);
                    setFileContent(null);
                    setEditMode(false);
                  }}
                  style={{
                    border: "none",
                    background: "none",
                    cursor: "pointer",
                    padding: "2px 6px",
                    fontSize: "14px",
                    color: "#64748b",
                  }}
                >
                  &#x2715;
                </button>
              </div>
            </div>
            <div style={{ flex: 1, overflow: "auto" }}>
              {editMode ? (
                <textarea
                  value={editContent}
                  onChange={(e) => setEditContent(e.target.value)}
                  style={{
                    width: "100%",
                    height: "100%",
                    border: "none",
                    outline: "none",
                    fontFamily: "'Menlo', 'Monaco', 'Courier New', monospace",
                    fontSize: "13px",
                    lineHeight: "1.5",
                    padding: "12px",
                    resize: "none",
                    backgroundColor: "#fff",
                  }}
                />
              ) : (
                <pre
                  style={{
                    margin: 0,
                    padding: "12px",
                    fontFamily: "'Menlo', 'Monaco', 'Courier New', monospace",
                    fontSize: "13px",
                    lineHeight: "1.5",
                    whiteSpace: "pre-wrap",
                    wordBreak: "break-all",
                  }}
                >
                  {fileContent}
                </pre>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
