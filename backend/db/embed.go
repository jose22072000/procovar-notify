// Package db expone los assets SQL (migraciones y, más adelante, seeds)
// embebidos en el binario, para que cmd/migrate y cmd/api los lleven consigo
// sin depender del filesystem en runtime.
package db

import "embed"

// Migrations contiene los .sql de goose.
//
//go:embed migrations/*.sql
var Migrations embed.FS
