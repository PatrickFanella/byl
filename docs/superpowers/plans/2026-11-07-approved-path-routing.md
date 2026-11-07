# Approved entrance and corridor routing

Issue #27, one PR. Preserve story stops while routing through reviewed entrances,
selected corridor traversals and non-story shaping points.

1. Build bounded routing controls from a consistent working itinerary. Require
   independently allowed arrivals whenever approved navigation is requested.
   Validate explicit segment topology and mode selection; retain repeated
   traversals and a declared return to the first stop. Reject ambiguous stop
   occurrences, missing mode paths and unattached endpoints.
2. Add a controlled-provider capability. Valhalla sends story points as `break`
   and intermediate controls as `via`. Check returned leg count, geometry and
   control order. Straight-line routing cannot satisfy approved navigation.
3. Persist stable story-leg identities and closing-leg metadata in existing JSON.
   Add schema 15 functions to fence changed arrival inputs and accurately record
   the coordinate source in new snapshots. Preserve existing hashes, cache rows
   and immutable revisions when no approved navigation is present. Require
   explicit publication of controlled routes.
4. Keep closing travel ahead in web/Android/iOS leg styling while story stops
   remain. Exercise synthetic provider failures, multiple shapes per story leg, separate
   modes, loops, out-and-back distance, stale arrival results and snapshot
   compatibility using real PostgreSQL. Update API and rollout documentation.
5. Compare the new adapter against the bounded DuPage Illinois Prairie Path
   reference already retrieved privately. Keep source geometry and real response
   artifacts outside Git. Record derived measurements and their limited scope.
6. Run relevant local checks; open one PR closing #27, link immediately, then
   watch its exact head. Merge after all checks pass. Qualify accepted SQL with
   a fresh independently restored production backup before migration admission;
   verify the actual release and public compatibility.

Provider geometry validation here detects malformed or ignored controls. It
does not establish current public access, grade separation or corridor-wide path
quality. Issue #29 owns the broader quality assessment. Production Valhalla and
automatic route publication are not enabled by this work.
