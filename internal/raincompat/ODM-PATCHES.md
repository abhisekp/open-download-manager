# ODM runtime compatibility patch

This is the MIT-licensed runtime package subset of `github.com/cenkalti/rain/v2` v2.5.1, already used by ODM. The upstream LICENSE is retained. `UPSTREAM-FILES.json` records the original module checksum and exact pre-patch file hashes. Packages were selected from actual dependencies across ODM's nine supported Go targets; all non-test Go files in those directories were copied unchanged before applying the patch. The upstream CLI, images, torrent samples and test fixtures are not included. This does not copy MDM/FDM/IDM code or change ODM's MIT application license.

## Reviewed changes

- `internal/announcer/periodic.go`: track spawned announcement workers and wait for them after the loop has closed. Previously `Close` waited only for the loop, allowing an in-flight `getTorrent` snapshot to race with torrent `closeData` clearing its piece structures. Waiting after the loop also prevents future worker additions from racing with Wait.
- `internal/announcer/odm_shutdown_test.go`: deterministic blocked-snapshot regression. Close must remain blocked until that snapshot is released, and must then finish on cancellation without contacting a real tracker.
- `go.mod`/`go.sum`: retain only dependencies used by the copied runtime and the same original MIT nettools compatibility replacement as the parent ODM module, so focused module tests use the reviewed implementation.

The main module retains its pinned v2.5.1 requirement and explicitly replaces it with this local source. License inventories and SBOMs identify the replacement. Run the focused race test plus ODM's real local tracker edit/removal and peer-transfer tests. This is a narrow maintained patch, not a claim that every upstream test or public swarm has been validated. Prefer an upstream release containing the fix after compatibility review.

Upstream: https://github.com/cenkalti/rain . The defect was observed in ODM core workflow 44697753458; no external issue or message was posted on the owner's behalf.
