#!/usr/bin/env python3
"""Check repository Markdown links and website assets/anchors without network access."""
from html.parser import HTMLParser
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parent.parent
REPO_URL = 'https://github.com/mdelapenya/biomelab/blob/main/'


class HTMLLinks(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.links = []
        self.ids = set()
        self.feed(text)

    def handle_starttag(self, tag, attrs):
        for key, value in attrs:
            if key == 'id':
                self.ids.add(value)
            if key in ('href', 'src', 'data-src-dark', 'data-src-light') and value:
                self.links.append(value)


def markdown(text):
    # Code examples can contain paths that are not documentation links.
    text = re.sub(r'^```.*?^```\s*$', '', text, flags=re.M | re.S)
    links = re.findall(r'!?\[[^\]\n]*\]\(([^\s)]+)(?:\s+"[^"]*")?\)', text)
    ids = set()
    counts = {}
    for title in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', text, re.M):
        slug = re.sub(r'[^\w\- ]', '', title.lower()).replace(' ', '-')
        count = counts.get(slug, 0)
        ids.add(slug if count == 0 else f'{slug}-{count}')
        counts[slug] = count + 1
    return links, ids


sources = sorted(set(ROOT.glob('*.md')) | set((ROOT / 'docs').rglob('*.md')) |
                 set((ROOT / '.claude').rglob('*.md')) | set((ROOT / 'website').rglob('*.html')))
errors = []
checked = 0
for source in sources:
    text = source.read_text()
    links = HTMLLinks(text).links if source.suffix == '.html' else markdown(text)[0]
    for link in links:
        if link.startswith(REPO_URL):
            target_url = urlsplit(link[len(REPO_URL):])
            target = ROOT / unquote(target_url.path)
        else:
            target_url = urlsplit(link)
            if target_url.scheme or target_url.netloc:
                continue
            path = unquote(target_url.path)
            target = (ROOT / 'website' / path.lstrip('/')) if path.startswith('/') else source.parent / path
            if not path:
                target = source
        checked += 1
        if not target.exists():
            errors.append(f'{source.relative_to(ROOT)}: missing target {link}')
            continue
        if target_url.fragment and target.suffix in ('.md', '.html'):
            content = target.read_text()
            ids = HTMLLinks(content).ids if target.suffix == '.html' else markdown(content)[1]
            if unquote(target_url.fragment) not in ids:
                errors.append(f'{source.relative_to(ROOT)}: missing anchor {link}')

for error in errors:
    print(error, file=sys.stderr)
print(f'Checked {checked} local links/assets in {len(sources)} documents; {len(errors)} errors.')
sys.exit(bool(errors))
