# Lore website — manual Cloudflare upload

Upload the contents of this folder to your existing loredev.co Cloudflare Pages project. This is a static site: no build command, dependencies, or secrets are needed.

1. Keep index.html at the root alongside the other HTML files, styles.css, site.js and favicon.png.
2. In your Cloudflare Pages project, choose Create deployment / Upload assets. Upload this folder or the supplied lore-website-upload.zip (the files are at the ZIP root).
3. Preview the deployment, check the installation links, then confirm the existing loredev.co custom domain serves it.

The _headers file supplies Pages security headers. If your existing deployment is a Worker rather than Pages, use its static-assets deployment flow; uploading HTML alone does not configure a Worker.

Files: homepage, setup guide, changelog, privacy, terms, shared CSS/JS, original favicon, robots.txt, sitemap.xml and _headers. No build artifacts or private app source are needed. The browser extension is explicitly a developer preview, with manual unpacked installation and visible-message capture limits.

Optional local validation: python verify.py. Optional preview from the parent folder: python -m http.server 4178 --directory lore-website.
