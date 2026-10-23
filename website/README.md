# Lore website — Windows alpha.24

Upload the verified `lore-website-upload-alpha24` folder to the existing
loredev.co Cloudflare Pages project. Keep `index.html` at the root and include
the `downloads/` folder. Cloudflare Pages Direct Upload accepts a whole folder.

Before publishing, run the alpha.24 upload verifier from the release folder.
After publishing, check the home and setup pages, download both files, compare
their SHA-256 hashes with `downloads/checksums.txt`, and try installation from
the public download on a clean Windows PC when one is available.

The release contains a native Windows MCP executable, a Claude Desktop `.mcpb`
extension, license, notices, and checksums. It contains no private chat archive
or application source. The native executable is unsigned; the site states this
limitation and directs users to keep Windows protections enabled.
