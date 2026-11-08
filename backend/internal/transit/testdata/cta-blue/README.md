# Recorded CTA Blue Line subset

These rows were extracted from CTA's official `google_transit.zip`, downloaded
on November 8, 2026. `provenance.json` records the exact source URL, retrieval
instant, full-archive SHA-256, license URL, service date and trip identifier.

The subset contains one Sunday trip and its Clark/Lake (Subway) and Forest Park
platforms, parent stations, calendar and associated table headers. It also retains
Clark/Lake's elevated platform `30074` to check that a real CTA platform on
another rail line is rejected. Header-only frequency/exception tables preserve
the captured source schema. Tests construct a ZIP from these tables; that small
ZIP's hash differs from the recorded full-archive hash by design.
Extracted CSV files use UTF-8 without a BOM and normalized LF line endings; the
recorded full-archive hash refers to the untouched downloaded ZIP.

The candidate is scheduled service for its recorded date. It does not establish
actual arrivals, station access, walking duration, bicycle eligibility or field
performance. Invalid calendars, pickup rules, GTFS times and malformed source
cases are explicit in-memory mutations in `cta_test.go`. HTTP/persistence tests
use separately labeled synthetic data.

Source: [CTA GTFS documentation](https://www.transitchicago.com/developers/gtfs/),
[official archive](https://www.transitchicago.com/downloads/sch_data/google_transit.zip),
[Developer License Agreement and Terms of Use](https://www.transitchicago.com/developers/terms/).
