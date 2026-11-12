# Byl design-system integration

Byl's selected system is **Street corner / Classic solid**, with the **BB1-4A northwest centre pair** for product use and the **P3 Oxblood** interface. Studio owns the identity, font selection, tokens and outlined masters. This repository owns the implementation.

The source pack is `subcult-tv/studio/brands/byl`, following the Studio consolidation. At import it was an uncommitted pack on Studio revision `d633a86a4f16b30811a7528eeab1864e389e50c8`. The revision alone does not identify those bytes. [The asset receipt](asset-receipt.json) records the exact source and destination hashes for all 44 selected files. No Studio reference boards, private catalogs, campaign drafts or ZIP bundles are included in the app output.

## Consumer mapping

| Surface | Implementation |
| --- | --- |
| Visitor web | Shared semantic CSS, local fonts, outlined masthead, labelled compact navigation, editorial headings, opaque image captions, story reading column, forms, status and arrival panels. |
| Curator web | Same color roles and Public Sans hierarchy across queue, imports, places, media, tours and jobs; a 240px navigation column at wide widths. |
| Web appearance | System by default. Light/Dark override persists in `byl.appearance`, separately from credentials. The head script applies explicit appearance before first paint. Changes update map styling without recreating the active tour. |
| Android | Generated Compose palette, bundled font families, BYL control shapes, theme-aware map layers and halos, opaque overlays, local appearance preference, outlined masthead and adaptive launcher/monochrome icons. |
| iOS | Generated light/dark color assets, Dynamic Type custom fonts, scene-level appearance, shared components and sheets, outlined masthead and light/dark launcher icons. |
| Shared-link fallback | Byl display copy and system-aware Oxblood styling. The standalone Go response uses Georgia/system font fallbacks because it does not serve the web font assets. |

The display name is Byl. Existing bundle/application IDs, API addresses, storage partitions, `/t/{slug}` links and `invisiblemuseum://tour/{slug}` remain the product's technical identifiers. This integration does not announce a new hostname, store listing or launch date.

## Refresh and verification

Run from the repository root:

```sh
python3 scripts/sync-byl.py --source <subcult-tv checkout>/studio/brands/byl
python3 scripts/generate-byl-palette.py
python3 scripts/sync-byl.py --check
python3 scripts/generate-byl-palette.py --check
```

The importer uses an explicit asset selection and refuses changes to previously imported consumer files. Review later Studio changes before updating the selection. Fonts retain their SIL Open Font License notices in web assets, the iOS resource folder and Android license assets. Native palette generation reads the imported `tokens.json`; edit the Studio source to change shared values.

The CI web job checks imported hashes and generated palettes. `web/e2e/appearance.e2e.ts` checks system appearance, persisted overrides, navigation, local fonts, focus, 320px/1440px layouts, 200% text size and active-tour state through appearance changes. Existing tests exercise the visitor and curator journeys against a temporary API/database.

## Local evidence — November 5, 2026

- Web type check and lint pass. All 64 unit tests and all 20 browser tests pass. The production static build passes.
- Backend tests pass against disposable PostgreSQL, including the shared-link fallback and existing URL-scheme checks.
- Android core tests, debug assembly and lint pass. Lint retains dependency/version, provider and unused-resource warnings; no errors remain.
- Android emulator rendering was inspected in Light and Dark; changing appearance updates the UI immediately and retains the choice on relaunch. The emulator's existing development API override points to an unavailable local server, so this check covers rendering and appearance, not native tour playback. Its API setting was left in place and appearance was returned to System.
- The selected asset check passes for 44 files. All 24 generated native palette outputs match the imported tokens.
- The advisory UI scanner inspected 94 source files: P0 findings fell from 3 to 0. It still reports 38 P1 hints: per-file focus detection does not see the global focus rule, and two arrow-glyph hints concern existing distance/review text. Browser checks establish the 3px focus outline; the scanner does not establish accessibility conformance.

The [screenshots](screenshots/) show the integrated web profile in both themes at compact and wide widths, plus the Android appearance screen. They are app captures, not Studio mockups.

iOS compilation and package tests require macOS/Xcode and are covered by the existing hosted CI job. No local iOS build or simulator result is claimed. Physical-device GPS, audio interruption, provider sign-in, VoiceOver/TalkBack, outdoor readability and native large-text journeys remain release qualification checks. No deployment or store publication is part of this integration.

## Editorial layout pass — November 12, 2026

A Studio review compared the live front page with the pack's reference boards. The identity, tokens, fonts and both appearances were already in place. The page hierarchy was not: page headings were 30px, section headings 18px, tours and stops sat in bordered cards, and buttons used 14px regular text.

The visitor pages now follow `design-system/FOUNDATIONS.md` and `COMPONENTS.md`:

- Headings use the token scale (32/40 growing to 40/48 at 960px, 28/36 sections, 22/30 items; the home opening uses the 56/64 display size at 960px). Metadata labels are small uppercase in the secondary text role.
- Sections are separated by rules with 48px (64px wide) spacing. Home places tours and nearby places on the eight/four-column grid.
- Tours are ruled rows. The stop index uses 32px numbered outline markers, ruled rows and each stop's own photograph.
- Tour detail shows its title, summary and estimates as a page heading. On compact screens the map precedes the actions and the stop list.
- Buttons are 48px, 16px Public Sans Bold with the control border. Topic and mode filters are 40px chips with a check and `aria-pressed`. The current navigation item uses weight and an indicator.
- Empty and error states use opaque panels. Photographs and the map have square corners.
- The home page title is "Byl · Local history, on foot or by bike".

The shared classes are in `web/src/routes/layout.css` under "Platform Index editorial layer". Route builder, report and suggestion forms, the transit panel and the curator pages keep their earlier component styling apart from the shared heading and label classes.

Captures from a local production build against a seeded local API: `screenshots/web-home-*` and `screenshots/web-tour-*` (before and after at 1440px light, after at 390px dark). The local API had not computed routes, so the estimates read "Route being computed".
