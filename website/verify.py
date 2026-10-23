"""Verify the exact Cloudflare upload folder for alpha.24."""

from hashlib import sha256
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlsplit
from zipfile import ZipFile
import json
import sys


root = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path(__file__).resolve().parent


class Page(HTMLParser):
    def __init__(self, source):
        super().__init__(convert_charrefs=True)
        self.ids = set()
        self.links = []
        self.feed(source)

    def handle_starttag(self, tag, attributes):
        attrs = dict(attributes)
        if "id" in attrs:
            self.ids.add(attrs["id"])
        self.links.extend(attrs[key] for key in ("href", "src") if key in attrs)


pages = {path.name: Page(path.read_text(encoding="utf-8")) for path in root.glob("*.html")}
assert set(pages) == {"index.html", "docs.html", "changelog.html", "privacy.html", "terms.html"}
for name, page in pages.items():
    for link in page.links:
        url = urlsplit(link)
        if url.scheme or url.netloc:
            continue
        target = url.path or name
        assert (root / target).is_file(), (name, link, "missing file")
        if url.fragment:
            assert target in pages and url.fragment in pages[target].ids, (name, link, "missing anchor")

downloads = root / "downloads"
checksums = {}
for line in (downloads / "checksums.txt").read_text(encoding="ascii").splitlines():
    digest, name = line.split()
    checksums[name] = digest
assert set(checksums) == {"lore-mcp.exe", "lore-memory-windows.mcpb"}
for name, expected in checksums.items():
    assert sha256((downloads / name).read_bytes()).hexdigest() == expected, name

with ZipFile(downloads / "lore-memory-windows.mcpb") as bundle:
    assert bundle.testzip() is None
    assert json.loads(bundle.read("manifest.json"))["version"] == "0.10.0-alpha.24"
    assert bundle.read("server/lore-mcp.exe") == (downloads / "lore-mcp.exe").read_bytes()

assert "alpha.24" in (root / "docs.html").read_text(encoding="utf-8")
assert "Connect Lore to all my coding apps" in (root / "docs.html").read_text(encoding="utf-8")
assert "alpha.23" not in (root / "downloads" / "README.md").read_text(encoding="utf-8")
print("PASS: alpha.24 upload pages, links, checksums, and embedded MCP server")
