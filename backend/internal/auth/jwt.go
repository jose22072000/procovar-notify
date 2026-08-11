package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Tipos de token. El access es de vida corta; el refresh sirve para renovarlo.
const (
	tokenTypeAccess  = "access"
	tokenTypeRefresh = "refresh"
)

// adminClaims son los claims del JWT de administración.
type adminClaims struct {
	jwt.RegisteredClaims
	Role          string `json:"role"`
	ApplicationID string `json:"app_id,omitempty"`
	TokenType     string `json:"typ"`
	// Version es la token_version del admin con la que se emitió el refresh; el
	// endpoint de refresh la compara con la BD para poder revocar sesiones.
	Version int `json:"ver,omitempty"`
}

// TokenIssuer emite y valida JWTs de administración (HS256).
type TokenIssuer struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewTokenIssuer crea el emisor con los TTL por defecto (15 min / 7 días).
func NewTokenIssuer(secret string) *TokenIssuer {
	return &TokenIssuer{
		secret:     []byte(secret),
		accessTTL:  15 * time.Minute,
		refreshTTL: 7 * 24 * time.Hour,
		now:        time.Now,
	}
}

// Issue emite un par (access, refresh) para el admin dado. tokenVersion es la
// token_version actual del admin: solo se graba en el refresh (el access es de
// vida corta y no se revisa por request, para mantener el hot-path stateless).
func (ti *TokenIssuer) Issue(admin Admin, tokenVersion int) (access, refresh string, err error) {
	access, err = ti.sign(admin, tokenTypeAccess, ti.accessTTL, 0)
	if err != nil {
		return "", "", err
	}
	refresh, err = ti.sign(admin, tokenTypeRefresh, ti.refreshTTL, tokenVersion)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

func (ti *TokenIssuer) sign(admin Admin, tokenType string, ttl time.Duration, version int) (string, error) {
	now := ti.now()
	claims := adminClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   admin.ID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Role:      admin.Role,
		TokenType: tokenType,
		Version:   version,
	}
	if admin.ApplicationID != nil {
		claims.ApplicationID = admin.ApplicationID.String()
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(ti.secret)
}

// parse valida la firma/expiración y el tipo de token esperado, y reconstruye
// la identidad del admin junto con la token_version grabada en el token.
func (ti *TokenIssuer) parse(tokenStr, expectedType string) (Admin, int, error) {
	var claims adminClaims
	_, err := jwt.ParseWithClaims(tokenStr, &claims, func(_ *jwt.Token) (any, error) {
		return ti.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return Admin{}, 0, fmt.Errorf("token inválido: %w", err)
	}
	if claims.TokenType != expectedType {
		return Admin{}, 0, fmt.Errorf("tipo de token inesperado: %s", claims.TokenType)
	}

	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Admin{}, 0, fmt.Errorf("subject inválido: %w", err)
	}
	admin := Admin{ID: id, Role: claims.Role}
	if claims.ApplicationID != "" {
		appID, err := uuid.Parse(claims.ApplicationID)
		if err != nil {
			return Admin{}, 0, fmt.Errorf("app_id inválido: %w", err)
		}
		admin.ApplicationID = &appID
	}
	return admin, claims.Version, nil
}

// ParseAccess valida un access token.
func (ti *TokenIssuer) ParseAccess(tokenStr string) (Admin, error) {
	admin, _, err := ti.parse(tokenStr, tokenTypeAccess)
	return admin, err
}

// ParseRefresh valida un refresh token y devuelve la token_version con la que se
// emitió (para compararla con la BD y poder revocar).
func (ti *TokenIssuer) ParseRefresh(tokenStr string) (Admin, int, error) {
	return ti.parse(tokenStr, tokenTypeRefresh)
}
