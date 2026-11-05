.PHONY: up down test test-db tidy vet

up:
	docker compose up --build

down:
	docker compose down

# Runs unit tests only unless TEST_DATABASE_URL is set.
test:
	go test -count=1 ./...

# Starts a throwaway Postgres, runs every test against it, and tears it down.
test-db:
	docker run -d --rm --name museum-test-pg -e POSTGRES_PASSWORD=museum -e POSTGRES_USER=museum -e POSTGRES_DB=museum_test -p 55432:5432 postgres:16-alpine >/dev/null
	@until docker exec museum-test-pg pg_isready -U museum -d museum_test >/dev/null 2>&1; do sleep 1; done
	TEST_DATABASE_URL="postgres://museum:museum@localhost:55432/museum_test?sslmode=disable" go test -count=1 ./... ; status=$$?; docker stop museum-test-pg >/dev/null; exit $$status

vet:
	go vet ./...

tidy:
	go mod tidy
