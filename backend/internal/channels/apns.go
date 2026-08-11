package channels

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// apnsTokenTTL es la vigencia con la que se cachea el JWT de APNs (Apple admite
// hasta 1 h; se usa un margen para no apurar la expiración).
const apnsTokenTTL = 50 * time.Minute

// tokenCache cachea tokens firmados por clave, con expiración. Thread-safe.
type tokenCache struct {
	mu sync.Mutex
	m  map[string]cachedToken
}

type cachedToken struct {
	token string
	exp   time.Time
}

func newTokenCache() *tokenCache { return &tokenCache{m: make(map[string]cachedToken)} }

func (c *tokenCache) get(key string, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[key]
	if !ok || !now.Before(t.exp) {
		return "", false
	}
	return t.token, true
}

func (c *tokenCache) set(key, token string, exp time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = cachedToken{token: token, exp: exp}
}

func (c *tokenCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

// sendAPNS envía una notificación push a Apple (APNs) con autenticación basada
// en token JWT (ES256 firmado con la clave .p8). Config esperada:
//
//	teamId, keyId, bundleId, privateKey (PEM .p8)[, endpoint][, production]
//
// El token JWT se firma una vez por (teamId,keyId) y se cachea ~50 min
// (apnsTokenTTL); Apple admite reusarlo hasta 1 h.
func (s *PushSender) sendAPNS(ctx context.Context, msg RenderedMessage, p ProviderConfig) (ProviderRef, error) {
	teamID := p.String("teamId")
	keyID := p.String("keyId")
	bundleID := p.String("bundleId")
	pemKey := p.String("privateKey")
	if teamID == "" || keyID == "" || bundleID == "" || pemKey == "" {
		return "", fmt.Errorf("apns: faltan teamId/keyId/bundleId/privateKey en la config")
	}

	// El token JWT se cachea ~50 min por (teamId,keyId): APNs admite reusarlo
	// hasta 1 h y penaliza regenerarlo con demasiada frecuencia.
	cacheKey := teamID + ":" + keyID
	now := time.Now()
	jwt, ok := s.apns.get(cacheKey, now)
	if !ok {
		key, err := parseP8(pemKey)
		if err != nil {
			return "", fmt.Errorf("apns: clave inválida: %w", err)
		}
		jwt, err = apnsJWT(teamID, keyID, key, now)
		if err != nil {
			return "", fmt.Errorf("apns: firmando token: %w", err)
		}
		s.apns.set(cacheKey, jwt, now.Add(apnsTokenTTL))
	}

	endpoint := p.String("endpoint")
	if endpoint == "" {
		// production=false usa el entorno de sandbox de APNs.
		if prod, _ := p.Config["production"].(bool); prod {
			endpoint = "https://api.push.apple.com"
		} else {
			endpoint = "https://api.sandbox.push.apple.com"
		}
	}
	url := fmt.Sprintf("%s/3/device/%s", strings.TrimRight(endpoint, "/"), msg.Recipient.PushToken)

	payload := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{"title": msg.Subject, "body": messageText(msg)},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(raw)))
	if err != nil {
		return "", err
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", bundleID)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("content-type", "application/json")

	res, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("apns: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var e struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Reason != "" {
			return "", fmt.Errorf("apns: status %d: %s", res.StatusCode, e.Reason)
		}
		return "", fmt.Errorf("apns: status %d", res.StatusCode)
	}
	// APNs devuelve el id del mensaje en la cabecera apns-id.
	return ProviderRef(res.Header.Get("apns-id")), nil
}

// parseP8 decodifica una clave privada EC de Apple (.p8, PKCS#8 PEM).
func parseP8(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemStr)))
	if block == nil {
		return nil, fmt.Errorf("PEM no válido")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("la clave no es ECDSA")
	}
	return key, nil
}

// apnsJWT construye y firma el JWT ES256 que APNs exige (iss=teamId, kid=keyId).
func apnsJWT(teamID, keyID string, key *ecdsa.PrivateKey, now time.Time) (string, error) {
	header := map[string]string{"alg": "ES256", "kid": keyID, "typ": "JWT"}
	claims := map[string]any{"iss": teamID, "iat": now.Unix()}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64(hb) + "." + b64(cb)

	digest := sha256.Sum256([]byte(signingInput))
	r, sgn, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	// Firma JWS ES256: r||s, cada uno relleno a 32 bytes (curva P-256).
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	sgn.FillBytes(sig[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
