package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
)

func generateUUIDKey(data []byte) uuid.UUID {
	return uuid.NewHash(sha256.New(), uuid.NameSpaceDNS, data, 5)
}

// Currently this is used to provide a proper migration for changes made in: https://github.com/guacsec/guac/pull/2060 and https://github.com/guacsec/guac/pull/2021.
// This changes to GUAC are a breaking change to existing ENT databases. This will provide a proper migration path before atlas is run.
func main() {
	conn, err := pgx.Connect(context.Background(), "postgres://guac:guac@localhost:5432/guac?sslmode=disable")
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer conn.Close(context.Background())

	// Step 1: Update the dependencies table by setting dependent_package_version_id
	_, err = conn.Exec(context.Background(), `
		UPDATE public.dependencies d
		SET dependent_package_version_id = pv.id
		FROM public.package_versions pv
		WHERE d.dependent_package_name_id IS NOT NULL
		  AND d.dependent_package_version_id IS NULL
		  AND d.dependent_package_name_id = pv.name_id
		  AND d.version_range = pv.version
	`)
	if err != nil {
		log.Fatalf("Failed to update dependent_package_version_id: %v\n", err)
	}

	// Temporarily disable foreign key constraints
	_, err = conn.Exec(context.Background(), `
		ALTER TABLE bill_of_materials_included_dependencies DROP CONSTRAINT bill_of_materials_included_dependencies_dependency_id;
	`)
	if err != nil {
		log.Fatalf("Failed to drop foreign key constraint: %v\n", err)
	}

	// Step 2: Generate new UUIDs for the id field in the dependencies table
	rows, err := conn.Query(context.Background(), `
		SELECT id, package_id, dependent_package_version_id, dependency_type, justification, origin, collector, document_ref
		FROM public.dependencies
	`)
	if err != nil {
		log.Fatalf("Failed to query dependencies: %v\n", err)
	}
	defer rows.Close()

	type Dependency struct {
		oldID           uuid.UUID
		newID           uuid.UUID
		packageID       uuid.UUID
		depPkgVersionID uuid.UUID
		dependencyType  string
		justification   string
		origin          string
		collector       string
		documentRef     string
	}

	var dependencies []Dependency

	for rows.Next() {
		var dep Dependency

		err := rows.Scan(&dep.oldID, &dep.packageID, &dep.depPkgVersionID, &dep.dependencyType, &dep.justification, &dep.origin, &dep.collector, &dep.documentRef)
		if err != nil {
			log.Fatalf("Failed to scan row: %v\n", err)
		}

		depIDString := fmt.Sprintf("%s::%s::%s::%s::%s::%s:%s?", dep.packageID.String(), dep.depPkgVersionID.String(), dep.dependencyType, dep.justification, dep.origin, dep.collector, dep.documentRef)
		dep.newID = generateUUIDKey([]byte(depIDString))

		dependencies = append(dependencies, dep)
	}

	batch := &pgx.Batch{}

	for _, dep := range dependencies {
		batch.Queue("UPDATE public.dependencies SET id = $1 WHERE id = $2", dep.newID, dep.oldID)
	}

	br := conn.SendBatch(context.Background(), batch)
	err = br.Close()
	if err != nil {
		log.Fatalf("Failed to update dependencies with new UUIDs: %v\n", err)
	}

	// Step 3: Update the related tables to reference the new UUIDs
	batch = &pgx.Batch{}

	for _, dep := range dependencies {
		batch.Queue("UPDATE bill_of_materials_included_dependencies SET dependency_id = $1 WHERE dependency_id = $2", dep.newID, dep.oldID)
	}

	br = conn.SendBatch(context.Background(), batch)
	err = br.Close()
	if err != nil {
		log.Fatalf("Failed to update related tables with new UUIDs: %v\n", err)
	}

	// Re-enable foreign key constraints
	_, err = conn.Exec(context.Background(), `
		ALTER TABLE bill_of_materials_included_dependencies ADD CONSTRAINT bill_of_materials_included_dependencies_dependency_id FOREIGN KEY (dependency_id) REFERENCES dependencies(id) ON DELETE CASCADE;
	`)
	if err != nil {
		log.Fatalf("Failed to add foreign key constraint: %v\n", err)
	}
}
