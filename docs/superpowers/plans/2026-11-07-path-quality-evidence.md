# Path quality and access evidence

Issue #29, one PR. Preserve existing immutable public tours while requiring a
structured quality result for newly computed working routes and publication.

1. Extend the provider boundary with bounded path attribution. Valhalla inspects
   its returned geometry using `trace_attributes` with `edge_walk` only. Retain
   way/node identities, direction, mode, use and bridge/tunnel assertions. Missing
   attribution remains unknown; there is no map-snap or alternate-router retry.
2. Extend reviewed corridor references with explicit network identity and source
   evidence for access/connectivity. Resolve inspected source snapshots from the
   existing evidence store. Quotes/retrieval identifiers prove traceability, not
   semantic truth or real-time conditions. Ordinary source links and provider
   assertions must not be labelled inspected evidence.
3. Produce a bounded per-mode report tied to the exact route/input hash. Check
   selected way identities and explicit connectors, node connectivity including
   grade-separated crossings, travel direction/mode, closures and missing access
   evidence. Geometry proximity alone cannot pass. Development straight-line
   results cannot pass publication quality. Keep historical material editable.
4. Persist reports additively, fence stale results and expose private curator
   review details. Block publication of missing, stale or unresolved quality;
   preserve already published snapshots and active-trip data. Update the API,
   working-copy feedback, intake and recovery documentation as needed.
5. Test parallel-road, disconnected bridge, lawful connector, ambiguous access,
   closure, provider omission and development-route fixtures against PostgreSQL.
   Verify reruns and publication gates. Record bounded real comparisons for both
   named trail pilots: Illinois Prairie Path and Great Western Trail. Keep raw
   reference data private until copying rights are reviewed; report unknowns.
6. Run relevant local checks, open/link one PR closing #29 and watch its head.
   Merge only after all five CI jobs pass. Qualify any accepted SQL against a
   fresh independently restored production backup before admission, verify the
   release and continue dependency-ready issues.

Production Valhalla and public sign-in remain unconfigured. Passing a quality
report means the configured evidence checks are satisfied for curator review;
it does not certify current legal access, trail conditions or field performance.
