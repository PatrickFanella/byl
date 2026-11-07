# Versioned trail corridors and shaping points — issue 25

One PR adds reviewed, versioned route intent. Route generation that follows this
intent belongs to issue 27; this change must not claim that existing geometry
already follows an imported trail or has been field checked.

- Import bounded reference LineStrings with corridor identity, source URL/title,
  publisher, retrieval date, source version and license metadata. Retain an exact
  content hash. Each segment has an explicit ID, endpoint node IDs, branch IDs and
  independently allowed modes. Road connectors require their own source context.
- Record each mode's selected branch, direction, ordered segment traversals and
  explicit allowed connector IDs. Consecutive segments connect only through the
  declared endpoint node IDs. Nearby coordinates and visual crossings create no
  junction. Shared node coordinates must agree within the documented tolerance.
- Keep ordered shaping points separate from story stops. Tie them to a selected
  segment and adjacent stable story-stop IDs. They create no place, moment, stop
  or arrival event. Unselected/adjacent roads do not become allowed connectors.
- Use immutable curator proposals and expected-version approval/rejection, like
  reviewed arrivals. Approval changes the working draft and fences/requeues route
  jobs; explicit tour publication releases it. Retain choices during recompute.
  Recompute/legacy saves must not bypass the first explicit release of a version,
  and the initial pending public view must omit unreleased intent.
  A reviewed removal also advances the intent version. Changed stop/mode references
  need a corrected intent before publication, while old snapshots remain fixed.
- Add snapshot and route-input hash wrappers in a new migration. Preserve the
  legacy hash when no intent exists, so existing rows/routes stay unchanged.
  Add private curator APIs, public approved DTOs, OpenAPI and operator/intake docs.

Verify synthetic Prairie Path branch choices, an adjacent road, a separated
crossing, explicit connector round-trip, stop/shaping separation, stale review,
snapshot isolation and recompute retention on PostgreSQL. Compare complete old
public responses before/after migration with no intent. Require all five hosted
jobs and an exact accepted-source restore rehearsal before production admission.
