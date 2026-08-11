package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func testAdmin() Admin {
	appID := uuid.New()
	return Admin{ID: uuid.New(), Role: "APP_ADMIN", ApplicationID: &appID}
}

func TestJWTRoundtrip(t *testing.T) {
	ti := NewTokenIssuer("una-clave-secreta-larga-suficiente")
	admin := testAdmin()

	access, refresh, err := ti.Issue(admin, 3)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	got, err := ti.ParseAccess(access)
	if err != nil {
		t.Fatalf("parse access: %v", err)
	}
	if got.ID != admin.ID || got.Role != admin.Role || got.ApplicationID == nil || *got.ApplicationID != *admin.ApplicationID {
		t.Fatalf("identidad mal reconstruida: %+v", got)
	}
	gotRefresh, ver, err := ti.ParseRefresh(refresh)
	if err != nil {
		t.Fatalf("parse refresh: %v", err)
	}
	if gotRefresh.ID != admin.ID {
		t.Fatalf("identidad del refresh mal reconstruida: %+v", gotRefresh)
	}
	if ver != 3 {
		t.Fatalf("token_version esperada 3, got %d", ver)
	}
}

func TestJWTTypeConfusion(t *testing.T) {
	ti := NewTokenIssuer("clave")
	access, refresh, _ := ti.Issue(testAdmin(), 0)

	// Un access token no debe valer como refresh, ni viceversa.
	if _, _, err := ti.ParseRefresh(access); err == nil {
		t.Error("un access token no debería validar como refresh")
	}
	if _, err := ti.ParseAccess(refresh); err == nil {
		t.Error("un refresh token no debería validar como access")
	}
}

func TestJWTExpired(t *testing.T) {
	ti := NewTokenIssuer("clave")
	ti.now = func() time.Time { return time.Now().Add(-1 * time.Hour) } // emite ya caducado
	access, _, _ := ti.Issue(testAdmin(), 0)
	ti.now = time.Now

	if _, err := ti.ParseAccess(access); err == nil {
		t.Fatal("un token expirado debería rechazarse")
	}
}

func TestJWTTamperedSignature(t *testing.T) {
	ti := NewTokenIssuer("clave")
	access, _, _ := ti.Issue(testAdmin(), 0)

	// Cambiar el primer carácter del segmento de firma (bits significativos, no
	// padding) invalida la firma de forma fiable.
	parts := strings.Split(access, ".")
	sig := []byte(parts[2])
	if sig[0] == 'A' {
		sig[0] = 'B'
	} else {
		sig[0] = 'A'
	}
	tampered := parts[0] + "." + parts[1] + "." + string(sig)
	if _, err := ti.ParseAccess(tampered); err == nil {
		t.Fatal("una firma manipulada debería rechazarse")
	}
}

func TestJWTWrongSecret(t *testing.T) {
	issuer := NewTokenIssuer("clave-correcta")
	access, _, _ := issuer.Issue(testAdmin(), 0)

	other := NewTokenIssuer("clave-distinta")
	if _, err := other.ParseAccess(access); err == nil {
		t.Fatal("un token firmado con otra clave debería rechazarse")
	}
}

func TestJWTAlgNoneRejected(t *testing.T) {
	ti := NewTokenIssuer("clave")
	// Token con alg=none (intento de confusión de algoritmo).
	claims := adminClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Role:      "SUPER_ADMIN",
		TokenType: tokenTypeAccess,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("firmar none: %v", err)
	}
	if _, err := ti.ParseAccess(s); err == nil {
		t.Fatal("un token alg=none debería rechazarse (WithValidMethods HS256)")
	}
}
