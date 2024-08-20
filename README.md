# GUAC-Update-db

NOTE: Run this before running atlas migration!!!!

Currently this is used to provide a proper migration for changes made in: https://github.com/guacsec/guac/pull/2060 and https://github.com/guacsec/guac/pull/2021.

This changes to GUAC are a breaking change to existing ENT databases. This will provide a proper migration path before atlas is run.

Set the postgres environment variable `PGDATABASE`, `PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER`, and `PGPASSWORD` to set the address of the GUAC ENT Database