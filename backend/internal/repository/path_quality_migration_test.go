package repository

import "testing"

func TestPathQualityMigrationRetainsExistingRowsAndHashes(t *testing.T) {
	assertNavigationMigrationPreservesRows(t, 15)
}
