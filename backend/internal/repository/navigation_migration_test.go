package repository

import "testing"

func TestNavigationMigrationRetainsExistingRowsAndRouteHashes(t *testing.T) {
	assertNavigationMigrationPreservesRows(t, 14)
}
