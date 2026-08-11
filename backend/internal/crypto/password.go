package crypto

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parámetros Argon2id. Valores conservadores y razonables para login de admins.
// Se serializan en el hash, de modo que verificar funciona aunque cambien.
const (
	argonMemory      = 64 * 1024 // 64 MiB
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLen     = 16
	argonKeyLen      = 32
)

// ErrInvalidHash indica que el hash codificado no tiene el formato esperado.
var ErrInvalidHash = errors.New("crypto: hash con formato inválido")

// HashPassword deriva un hash Argon2id de la contraseña, en formato PHC
// ($argon2id$v=19$m=...,t=...,p=...$salt$hash), que incluye sal y parámetros.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("crypto: generando sal: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword comprueba una contraseña contra un hash PHC en tiempo constante.
func VerifyPassword(password, encoded string) (bool, error) {
	salt, hash, mem, iters, par, err := decodePHC(encoded)
	if err != nil {
		return false, err
	}
	computed := argon2.IDKey([]byte(password), salt, iters, mem, par, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, computed) == 1, nil
}

// decodePHC parsea el formato PHC de Argon2id y extrae sal, hash y parámetros.
func decodePHC(encoded string) (salt, hash []byte, mem uint32, iters uint32, par uint8, err error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}

	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iters, &par); err != nil {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}
	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}
	if hash, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return nil, nil, 0, 0, 0, ErrInvalidHash
	}
	return salt, hash, mem, iters, par, nil
}
