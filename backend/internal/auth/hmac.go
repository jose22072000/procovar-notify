// Package auth implementa la autenticación del servicio: firma HMAC para la API
// pública (consumida por los backends/tenants) y JWT para la API de
// administración (consumida por la SPA).
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
)

// Cabeceras de la autenticación HMAC de la API pública (§4.1 del diseño).
const (
	HeaderKeyID     = "X-QBN-Key-Id"
	HeaderTimestamp = "X-QBN-Timestamp"
	HeaderSignature = "X-QBN-Signature"
)

// CanonicalQuery devuelve la query en forma canónica (claves ordenadas y
// codificación RFC3986) para que cliente y servidor firmen exactamente lo mismo
// con independencia del orden en que vengan los parámetros. Cadena vacía si no
// hay query.
func CanonicalQuery(u *url.URL) string {
	return u.Query().Encode()
}

// StringToSign construye la cadena canónica que se firma:
//
//	METHOD \n PATH \n QUERY \n SHA256(body) \n TIMESTAMP
//
// La query (canónica) se incluye para que los parámetros queden autenticados
// (no solo el path); el hash del cuerpo cubre el payload.
func StringToSign(method, path, query string, body []byte, timestamp string) string {
	bodyHash := sha256.Sum256(body)
	return method + "\n" + path + "\n" + query + "\n" + hex.EncodeToString(bodyHash[:]) + "\n" + timestamp
}

// Sign calcula HMAC-SHA256(secret, StringToSign) y lo devuelve en hexadecimal.
// Lo usan tanto el cliente (para firmar) como el servidor (para recalcular).
func Sign(secret []byte, method, path, query string, body []byte, timestamp string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(StringToSign(method, path, query, body, timestamp)))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifySignature recalcula la firma y la compara con la provista en tiempo
// constante (hmac.Equal) para evitar ataques de temporización.
func verifySignature(secret []byte, method, path, query string, body []byte, timestamp, provided string) bool {
	expected := Sign(secret, method, path, query, body, timestamp)
	return hmac.Equal([]byte(expected), []byte(provided))
}
