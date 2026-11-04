package httpapi

import (
	"encoding/json"
	"io"
	"strings"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/testdb"
)

func stringsReader(s string) io.Reader { return strings.NewReader(s) }

func repository_CreateTourInput(vis models.TourVisibility, status models.EntityStatus, owner string) repository.CreateTourInput {
	return repository.CreateTourInput{
		CityID: testdb.SeedCityID, Title: "T " + string(vis) + " " + string(status), Modes: models.AllTravelModes,
		Status: status, Visibility: vis, OwnerAccountID: &owner, RandomSlug: true,
		Stops: []models.StopInput{{PlaceID: testdb.SeedPlaceStationID}, {PlaceID: testdb.SeedPlaceTivoliID}},
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
