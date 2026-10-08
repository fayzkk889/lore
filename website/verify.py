"""Check local site links, release checksums, and obsolete release claims."""
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlsplit
import hashlib

root = Path(__file__).resolve().parent


class Page(HTMLParser):
    def __init__(self, text):
        super().__init__(convert_charrefs=True)
        self.ids = set()
        self.links = []
        self.feed(text)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if "id" in attrs:
            self.ids.add(attrs["id"])
        for key in ("href", "src"):
            if key in attrs:
                self.links.append(attrs[key])


pages = {p.name: Page(p.read_text(encoding="utf-8")) for p in root.glob("*.html")}
assert set(pages) == {"index.html", "docs.html", "changelog.html", "privacy.html", "terms.html"}
for name, page in pages.items():
    for link in page.links:
        u = urlsplit(link)
        if u.scheme or u.netloc:
            continue
        target = u.path or name
        assert (root / target).is_file(), (name, link, "missing file")
        if u.fragment:
            assert target in pages and u.fragment in pages[target].ids, (name, link, "missing anchor")

expected_files = {
    "lore-mcp-linux",
    "install-codex.ps1",
    "LICENSE.txt",
    "THIRD_PARTY_NOTICES.txt",
    "checksums.txt",
}
assert {p.name for p in (root / "downloads").iterdir()} == expected_files
lines = (root / "downloads" / "checksums.txt").read_text(encoding="ascii").splitlines()
assert len(lines) == len(expected_files) - 1
for line in lines:
    expected, name = line.split()
    assert name in expected_files and name != "checksums.txt"
    actual = hashlib.sha256((root / "downloads" / name).read_bytes()).hexdigest()
    assert actual.casefold() == expected.casefold(), (name, "checksum mismatch")

content = "\n".join(p.read_text(encoding="utf-8") for p in root.glob("*.html"))
for stale in ("alpha.7_windows", "lore connect", "browser install", "extension preview ZIP"):
    assert stale not in content, ("obsolete public instruction", stale)
assert "Ubuntu WSL 2" in (root / "docs.html").read_text(encoding="utf-8")
print("LORE_MCP_SITE_OK", len(pages), "pages", len(lines), "verified downloads")
