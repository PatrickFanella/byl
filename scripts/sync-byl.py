#!/usr/bin/env python3
"""Import the selected Studio assets; --check verifies the recorded consumer bytes."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
RECEIPT = ROOT / 'docs/design/byl/asset-receipt.json'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    previous = json.loads(RECEIPT.read_text()) if RECEIPT.exists() else None
    if args.check:
        if not previous:
            raise SystemExit('No asset receipt')
        for item in previous['assets']:
            if digest((ROOT / item['target']).read_bytes()) != item['sha256']:
                raise SystemExit(f"Consumer drift: {item['target']}")
        print(f"PASS: {len(previous['assets'])} BYL asset hashes")
        return
    if not args.source:
        parser.error('--source must name Studio brands/byl')
    source = args.source.resolve()
    web = 'web/static/brand/'
    ios = 'ios-invisible-museum/InvisibleMuseum/'
    android = 'android/app/src/main/'
    mapping = [('tokens.json', 'docs/design/byl/tokens.json'), ('tokens.css', web + 'tokens.css')]
    for name in ['lockup-primary-color-outlined.svg', 'lockup-primary-reverse-outlined.svg', 'mark-primary-color.svg']:
        mapping.append(('logos/' + name, web + name))
    for name in ['favicon.svg', 'favicon-32.png', 'apple-touch-icon.png', 'icon-192.png', 'icon-512.png', 'maskable-512.png', 'link-preview.png']:
        mapping.append(('product/' + name, web + name))
    font_names = {'Public-Sans-0.ttf': 'public_sans_regular.ttf', 'Public-Sans-1.ttf': 'public_sans_bold.ttf',
                  'Libre-Baskerville-0.ttf': 'libre_baskerville.ttf', 'Barlow-Condensed-1.ttf': 'barlow_condensed_bold.ttf'}
    for name, native in font_names.items():
        for target in [web + 'fonts/' + name, ios + 'Fonts/' + name, android + 'res/font/' + native]:
            mapping.append(('fonts/' + name, target))
    for name in ['publicsans-OFL.txt', 'librebaskerville-OFL.txt', 'barlowcondensed-OFL.txt']:
        for target in [web + 'fonts/' + name, ios + 'Fonts/' + name, android + 'assets/licenses/' + name]:
            mapping.append(('fonts/' + name, target))
    mapping += [('product/app-icon-1024.png', ios + 'Assets.xcassets/AppIcon.appiconset/icon.png'),
                ('product/app-icon-dark-1024.png', ios + 'Assets.xcassets/AppIcon.appiconset/icon-dark.png'),
                ('logos/mark-primary-color.png', ios + 'Assets.xcassets/BylMark.imageset/mark.png'),
                ('logos/lockup-primary-color-outlined.png', ios + 'Assets.xcassets/BylLockup.imageset/lockup.png'),
                ('logos/lockup-primary-reverse-outlined.png', ios + 'Assets.xcassets/BylLockup.imageset/lockup-dark.png'),
                ('logos/lockup-primary-color-outlined.png', android + 'res/drawable-nodpi/byl_lockup.png'),
                ('logos/lockup-primary-reverse-outlined.png', android + 'res/drawable-nodpi/byl_lockup_dark.png'),
                ('product/android-adaptive-foreground.png', android + 'res/drawable-nodpi/byl_launcher_foreground.png'),
                ('product/android-monochrome.png', android + 'res/drawable-nodpi/byl_launcher_monochrome.png'),
                ('product/app-icon-1024.png', android + 'res/mipmap-nodpi/ic_launcher.png'),
                ('logos/mark-primary-color.png', android + 'res/drawable-nodpi/byl_mark.png')]
    previous_hashes = {a['target']: a['sha256'] for a in previous['assets']} if previous else {}
    assets = []
    # Refuse later consumer edits before writing any files.
    for _, target in mapping:
        path = ROOT / target
        if target in previous_hashes and path.exists() and digest(path.read_bytes()) != previous_hashes[target]:
            raise SystemExit(f'Consumer drift: {target}')
    for origin, target in mapping:
        data = (source / origin).read_bytes()
        path = ROOT / target
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        assets.append({'source': origin, 'target': target, 'sha256': digest(data), 'transformation': 'unchanged copy'})
    revision = subprocess.check_output(['git', '-C', str(source), 'rev-parse', 'HEAD'], text=True).strip()
    status = subprocess.check_output(['git', '-C', str(source), 'status', '--porcelain', '--', '.'], text=True).strip()
    RECEIPT.parent.mkdir(parents=True, exist_ok=True)
    RECEIPT.write_text(json.dumps({'source': 'subcult-tv/studio/brands/byl', 'studio_revision': revision,
                                 'source_state': 'uncommitted pack' if status else 'clean',
                                 'selection': 'Classic solid / BB1-4A product mark / P3 Oxblood', 'assets': assets}, indent=2) + '\n')
    print(f'Imported {len(assets)} selected BYL assets')


if __name__ == '__main__':
    main()
