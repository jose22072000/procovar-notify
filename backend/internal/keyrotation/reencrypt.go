// Package keyrotation re-cifra los secretos en reposo con la clave primaria
// actual del keyring (rotación KMS, Fase 21). Es idempotente: salta las filas
// ya cifradas con la versión primaria.
package keyrotation

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store"
)

// target describe una columna cifrada a migrar.
type target struct {
	Table  string
	Column string
}

// Targets son todas las columnas con secretos cifrados en reposo.
var Targets = []target{
	{"api_keys", "secret_enc"},
	{"smtp_connections", "password_enc"},
	{"channel_providers", "config_enc"},
	{"webhook_endpoints", "secret_enc"},
}

// Result son los contadores por columna de una re-encriptación.
type Result struct {
	Table   string
	Column  string
	Updated int
}

// Reencrypt recorre todas las columnas con secretos y las migra a la versión
// primaria del Encryptor dado. Devuelve los contadores por columna.
func Reencrypt(ctx context.Context, db *store.DB, enc *crypto.Encryptor) ([]Result, error) {
	primary := enc.PrimaryVersion()
	results := make([]Result, 0, len(Targets))
	for _, t := range Targets {
		n, err := reencryptColumn(ctx, db, enc, t, primary)
		if err != nil {
			return results, fmt.Errorf("%s.%s: %w", t.Table, t.Column, err)
		}
		results = append(results, Result{Table: t.Table, Column: t.Column, Updated: n})
	}
	return results, nil
}

// reencryptColumn descifra cada valor (con cualquier versión del keyring) y lo
// re-cifra con la primaria, saltando los que ya la usan.
func reencryptColumn(ctx context.Context, db *store.DB, enc *crypto.Encryptor, t target, primary byte) (int, error) {
	//nolint:gosec // table/column provienen de una lista fija interna, no de entrada externa.
	rows, err := db.Pool.Query(ctx, fmt.Sprintf("SELECT id, %s FROM %s", t.Column, t.Table))
	if err != nil {
		return 0, err
	}
	type item struct {
		id  uuid.UUID
		enc []byte
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.enc); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	count := 0
	for _, it := range items {
		if len(it.enc) > 0 && it.enc[0] == primary {
			continue // ya está en la versión primaria
		}
		plain, err := enc.Decrypt(it.enc)
		if err != nil {
			return count, fmt.Errorf("descifrando %s: %w", it.id, err)
		}
		reEnc, err := enc.Encrypt(plain)
		if err != nil {
			return count, err
		}
		//nolint:gosec // ver nota anterior sobre table/column.
		if _, err := db.Pool.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = $2", t.Table, t.Column), reEnc, it.id); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
