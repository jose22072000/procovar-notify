package keyrotation

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

func TestReencrypt_RotatesToPrimary(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()

	keyA := bytes.Repeat([]byte{0xA}, 32)
	keyB := bytes.Repeat([]byte{0xB}, 32)
	encA, _ := crypto.NewKeyring(1, map[byte][]byte{1: keyA})          // estado inicial (v1)
	ring, _ := crypto.NewKeyring(2, map[byte][]byte{1: keyA, 2: keyB}) // tras rotar (primaria v2)
	encBonly, _ := crypto.NewKeyring(2, map[byte][]byte{2: keyB})      // solo la clave nueva

	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	must(t, err)

	// SMTP con password cifrado con la clave vieja (v1).
	pwdEnc, _ := encA.Encrypt([]byte("smtp-pass"))
	var smtpID uuid.UUID
	must(t, db.Pool.QueryRow(ctx, `
		INSERT INTO smtp_connections (application_id, name, host, port, username, password_enc, from_email, from_name, secure)
		VALUES ($1,'main','localhost',587,'u',$2,'a@b.c','X',false) RETURNING id`, app.ID, pwdEnc).Scan(&smtpID))

	// API key con secret cifrado con la clave vieja (v1).
	secEnc, _ := encA.Encrypt([]byte("api-secret"))
	must(t, db.Pool.QueryRow(ctx, `
		INSERT INTO api_keys (application_id, key_id, secret_enc, scopes)
		VALUES ($1,'k_1',$2,ARRAY['notifications:send']) RETURNING id`, app.ID, secEnc).Scan(new(uuid.UUID)))

	// Confirma estado inicial: el byte de versión es 1.
	if firstByte(t, db, "smtp_connections", "password_enc", smtpID) != 1 {
		t.Fatal("el password debería empezar en versión 1")
	}

	// Re-cifra con el keyring rotado (primaria v2).
	results, err := Reencrypt(ctx, db, ring)
	must(t, err)
	updated := 0
	for _, r := range results {
		updated += r.Updated
	}
	if updated != 2 {
		t.Fatalf("deberían re-cifrarse 2 secretos, fueron %d", updated)
	}

	// Ahora todo está en versión 2 y se descifra SOLO con la clave nueva.
	if firstByte(t, db, "smtp_connections", "password_enc", smtpID) != 2 {
		t.Fatal("el password debería estar en versión 2 tras re-cifrar")
	}
	newEnc := readBytes(t, db, "smtp_connections", "password_enc", smtpID)
	plain, err := encBonly.Decrypt(newEnc)
	if err != nil || string(plain) != "smtp-pass" {
		t.Fatalf("la clave nueva debería descifrar el valor migrado: %q %v", plain, err)
	}

	// Idempotencia: una segunda pasada no re-cifra nada.
	results2, err := Reencrypt(ctx, db, ring)
	must(t, err)
	for _, r := range results2 {
		if r.Updated != 0 {
			t.Fatalf("la segunda pasada no debería re-cifrar (%s.%s = %d)", r.Table, r.Column, r.Updated)
		}
	}
}

func readBytes(t *testing.T, db *store.DB, table, column string, id uuid.UUID) []byte {
	t.Helper()
	var b []byte
	//nolint:gosec // table/column fijos en el test.
	if err := db.Pool.QueryRow(context.Background(), "SELECT "+column+" FROM "+table+" WHERE id = $1", id).Scan(&b); err != nil {
		t.Fatalf("leyendo %s.%s: %v", table, column, err)
	}
	return b
}

func firstByte(t *testing.T, db *store.DB, table, column string, id uuid.UUID) byte {
	t.Helper()
	b := readBytes(t, db, table, column, id)
	if len(b) == 0 {
		t.Fatalf("%s.%s vacío", table, column)
	}
	return b[0]
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}
