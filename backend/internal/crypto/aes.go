// Package crypto centraliza la criptografía del servicio: cifrado simétrico de
// secretos en reposo (SMTP/proveedor/API key) con AES-256-GCM, y hashing de
// contraseñas de administrador con Argon2id.
//
// Frontera de cifrado: este paquete es el ÚNICO que conoce la clave. El resto
// del sistema maneja ciphertext (bytea) por debajo de la capa de servicio y
// plaintext solo de forma transitoria en el punto de uso.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

// ErrDecrypt es el error genérico de descifrado (no se detalla la causa para no
// dar pistas a un atacante).
var ErrDecrypt = errors.New("crypto: descifrado fallido")

// Encryptor cifra y descifra secretos con AES-256-GCM soportando un keyring de
// varias versiones de clave (rotación KMS): cifra con la versión PRIMARIA y
// descifra con la versión etiquetada en cada ciphertext. El formato es
// [versión(1)] [nonce] [ciphertext+tag], compatible con datos previos (v1).
type Encryptor struct {
	primary byte                 // versión usada al cifrar
	keys    map[byte]cipher.AEAD // todas las versiones disponibles para descifrar
}

// NewEncryptor crea un Encryptor de una sola clave (versión 1). Equivale a un
// keyring con una única versión; mantiene compatibilidad con datos existentes.
func NewEncryptor(key []byte) (*Encryptor, error) {
	return NewKeyring(1, map[byte][]byte{1: key})
}

// NewKeyring crea un Encryptor con varias versiones de clave. primary es la
// versión con la que se cifra; keysByVersion debe incluirla y todas las antiguas
// necesarias para descifrar datos previos. Cada clave es de 32 bytes (AES-256).
func NewKeyring(primary byte, keysByVersion map[byte][]byte) (*Encryptor, error) {
	if _, ok := keysByVersion[primary]; !ok {
		return nil, fmt.Errorf("crypto: la clave primaria (versión %d) no está en el keyring", primary)
	}
	keys := make(map[byte]cipher.AEAD, len(keysByVersion))
	for v, key := range keysByVersion {
		if len(key) != 32 {
			return nil, fmt.Errorf("crypto: la clave versión %d debe ser de 32 bytes, son %d", v, len(key))
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("crypto: aes.NewCipher (v%d): %w", v, err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("crypto: cipher.NewGCM (v%d): %w", v, err)
		}
		keys[v] = gcm
	}
	return &Encryptor{primary: primary, keys: keys}, nil
}

// PrimaryVersion devuelve la versión de clave con la que se cifra actualmente.
func (e *Encryptor) PrimaryVersion() byte { return e.primary }

// Encrypt cifra plaintext con la clave primaria: [versión(1)] [nonce] [ciphertext+tag].
func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	gcm := e.keys[e.primary]
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: generando nonce: %w", err)
	}
	// Seal añade el ciphertext al primer argumento; reservamos el prefijo de
	// versión + nonce para no recopiar.
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+gcm.Overhead())
	out = append(out, e.primary)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plaintext, nil), nil
}

// Decrypt revierte Encrypt usando la clave de la versión etiquetada. Rechaza
// versiones que no estén en el keyring.
func (e *Encryptor) Decrypt(data []byte) ([]byte, error) {
	if len(data) < 1 {
		return nil, ErrDecrypt
	}
	gcm, ok := e.keys[data[0]]
	if !ok {
		return nil, fmt.Errorf("%w: versión de clave desconocida %d", ErrDecrypt, data[0])
	}
	nonceSize := gcm.NonceSize()
	if len(data) < 1+nonceSize {
		return nil, ErrDecrypt
	}
	nonce := data[1 : 1+nonceSize]
	ciphertext := data[1+nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
