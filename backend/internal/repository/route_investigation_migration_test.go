package repository

import "testing"

func TestRouteInvestigationMigrationRetainsExistingRowsAndHashes(t *testing.T) {
	assertNavigationMigrationPreservesRows(t, 16)
}
