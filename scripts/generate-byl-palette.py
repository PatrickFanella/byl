#!/usr/bin/env python3
"""Generate native palettes from the imported BYL token source."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def camel(value):
    parts = value.split('-')
    return parts[0] + ''.join(p.title() for p in parts[1:])


def outputs(tokens):
    result = {}
    assets = ROOT / 'ios-invisible-museum/InvisibleMuseum/Assets.xcassets'
    for role in tokens['themes']['light']:
        name = 'Byl' + ''.join(p.title() for p in role.split('-'))
        colors = []
        for mode in ['light', 'dark']:
            h = tokens['themes'][mode][role].lstrip('#')
            components = dict(zip(['red', 'green', 'blue'], [f'0x{h[i:i+2]}' for i in (0, 2, 4)]), alpha='1.000')
            entry = {'idiom': 'universal', 'color': {'color-space': 'srgb', 'components': components}}
            if mode == 'dark':
                entry['appearances'] = [{'appearance': 'luminosity', 'value': 'dark'}]
            colors.append(entry)
        result[assets / (name + '.colorset') / 'Contents.json'] = json.dumps(
            {'colors': colors, 'info': {'author': 'xcode', 'version': 1}}, indent=2) + '\n'
    text = '''// Generated from docs/design/byl/tokens.json by scripts/generate-byl-palette.py.
package tv.subcult.museum.ui.theme

import androidx.compose.ui.graphics.Color

data class BylPalette(
'''
    text += ',\n'.join(f'    val {camel(k)}: Color' for k in tokens['themes']['light']) + '\n)\n'
    for mode, values in tokens['themes'].items():
        text += f'\nval {mode.title()}BylPalette = BylPalette(\n'
        text += ',\n'.join(f'    {camel(k)} = Color(0xFF{v[1:].upper()})' for k, v in values.items()) + '\n)\n'
    result[ROOT / 'android/app/src/main/kotlin/tv/subcult/museum/ui/theme/BylPalette.kt'] = text
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    tokens = json.loads((ROOT / 'docs/design/byl/tokens.json').read_text())
    generated = outputs(tokens)
    for path, text in generated.items():
        if args.check:
            if not path.exists() or path.read_text() != text:
                raise SystemExit(f'Palette drift: {path.relative_to(ROOT)}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text)
    print(f"PASS: {len(generated)} native palette outputs {'match' if args.check else 'generated'}")


if __name__ == '__main__':
    main()
