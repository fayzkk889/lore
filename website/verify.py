"""Check local pages, assets, fragment links and setup-command formatting."""
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlsplit

root=Path(__file__).resolve().parent
class Page(HTMLParser):
    def __init__(self,text):
        super().__init__(convert_charrefs=True);self.ids=set();self.links=[];self.copies=[];self.feed(text)
    def handle_starttag(self,tag,attrs):
        attrs=dict(attrs)
        if 'id' in attrs:self.ids.add(attrs['id'])
        for key in ('href','src'):
            if key in attrs:self.links.append(attrs[key])
        if 'data-copy' in attrs:self.copies.append(attrs['data-copy'])
pages={p.name:Page(p.read_text(encoding='utf-8')) for p in root.glob('*.html')}
for name,page in pages.items():
    for link in page.links:
        u=urlsplit(link)
        if u.scheme or u.netloc:continue
        target=u.path or name
        assert (root/target).is_file(),(name,link,'missing file')
        if u.fragment:assert target in pages and u.fragment in pages[target].ids,(name,link,'missing anchor')
assert len(pages)==5
assert pages['docs.html'].copies[0].splitlines()==['Invoke-WebRequest https://raw.githubusercontent.com/fayzkk889/lore/main/install.ps1 -OutFile install-lore.ps1',r'.\install-lore.ps1']
assert 'No signup' not in (root/'index.html').read_text()
print('WEBSITE_LOCAL_LINKS_ASSETS_AND_COMMANDS_OK',len(pages),'pages')
