# Lore website — manual Cloudflare upload

Upload this entire folder to the existing loredev.co Cloudflare Pages project.
Keep index.html at the root and include the downloads folder. This is a static
site with no build command, dependencies, or secrets.

This version describes **0.10.0-alpha.9-mcp**, the free, closed-source Lore
developer preview for Codex on Windows via Ubuntu WSL 2. It replaces the
previous unsigned Windows executable and browser-extension links. The preview
is tested locally but is not a broad one-click Windows release. Other MCP
clients need separate validation.

Before uploading, run `python verify.py` in this folder. After uploading,
preview the Pages deployment and check the setup page plus the binary and
script download links before sending people to it. The `_headers` file
supplies Pages security headers. If your existing deployment is a Worker,
use its static-asset deployment flow.

The files under `downloads/` are the binary, Codex setup script, license,
third-party notices, and SHA-256 checksums. The binary is a Linux executable
for WSL, not a native Windows application. No app source or private memory
archive belongs in this folder.
