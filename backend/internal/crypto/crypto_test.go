package crypto

import (
	"bytes"
	"testing"
)

func newTestEncryptor(t *testing.T) *Encryptor {
	t.Helper()
	enc, err := NewEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	return enc
}

func TestNewEncryptorRejectsBadKey(t *testing.T) {
	if _, err := NewEncryptor(make([]byte, 16)); err == nil {
		t.Fatal("se esperaba error con clave de 16 bytes")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	enc := newTestEncryptor(t)
	plaintext := []byte("smtp-password-súper-secreta")

	ct, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(ct, plaintext) {
		t.Fatal("el ciphertext no debería contener el plaintext")
	}
	if ct[0] != enc.PrimaryVersion() {
		t.Fatalf("primer byte debería ser la versión de clave %d, fue %d", enc.PrimaryVersion(), ct[0])
	}

	got, err := enc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip falló: %q != %q", got, plaintext)
	}
}

func TestKeyringRotation(t *testing.T) {
	keyA := bytes.Repeat([]byte{0xA}, 32)
	keyB := bytes.Repeat([]byte{0xB}, 32)

	// Estado inicial: una sola clave versión 1.
	old, err := NewKeyring(1, map[byte][]byte{1: keyA})
	if err != nil {
		t.Fatalf("NewKeyring v1: %v", err)
	}
	ctV1, _ := old.Encrypt([]byte("secreto"))
	if ctV1[0] != 1 {
		t.Fatalf("debería etiquetar versión 1, fue %d", ctV1[0])
	}

	// Tras rotar: primaria v2=keyB, conserva v1=keyA para descifrar lo antiguo.
	ring, err := NewKeyring(2, map[byte][]byte{1: keyA, 2: keyB})
	if err != nil {
		t.Fatalf("NewKeyring rotado: %v", err)
	}
	// Descifra el ciphertext antiguo (v1)...
	got, err := ring.Decrypt(ctV1)
	if err != nil || string(got) != "secreto" {
		t.Fatalf("el keyring debería descifrar datos v1: %q %v", got, err)
	}
	// ...y cifra lo nuevo con v2.
	ctV2, _ := ring.Encrypt([]byte("nuevo"))
	if ctV2[0] != 2 {
		t.Fatalf("debería etiquetar versión 2, fue %d", ctV2[0])
	}
	// La clave vieja sola NO puede descifrar lo cifrado con v2.
	if _, err := old.Decrypt(ctV2); err == nil {
		t.Fatal("la clave antigua no debería descifrar datos v2")
	}
}

func TestNewKeyringRejectsMissingPrimary(t *testing.T) {
	if _, err := NewKeyring(2, map[byte][]byte{1: bytes.Repeat([]byte{1}, 32)}); err == nil {
		t.Fatal("se esperaba error: la primaria no está en el keyring")
	}
}

func TestEncryptIsNondeterministic(t *testing.T) {
	enc := newTestEncryptor(t)
	a, _ := enc.Encrypt([]byte("x"))
	b, _ := enc.Encrypt([]byte("x"))
	if bytes.Equal(a, b) {
		t.Fatal("dos cifrados del mismo texto deberían diferir (nonce aleatorio)")
	}
}

func TestDecryptRejectsTampered(t *testing.T) {
	enc := newTestEncryptor(t)
	ct, _ := enc.Encrypt([]byte("hola"))

	t.Run("byte alterado", func(t *testing.T) {
		bad := bytes.Clone(ct)
		bad[len(bad)-1] ^= 0xFF
		if _, err := enc.Decrypt(bad); err == nil {
			t.Fatal("se esperaba error con ciphertext alterado")
		}
	})

	t.Run("versión desconocida", func(t *testing.T) {
		bad := bytes.Clone(ct)
		bad[0] = 99
		if _, err := enc.Decrypt(bad); err == nil {
			t.Fatal("se esperaba error con versión de clave desconocida")
		}
	})

	t.Run("demasiado corto", func(t *testing.T) {
		if _, err := enc.Decrypt([]byte{enc.PrimaryVersion()}); err == nil {
			t.Fatal("se esperaba error con datos demasiado cortos")
		}
	})
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("contraseña-admin")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword("contraseña-admin", hash)
	if err != nil || !ok {
		t.Fatalf("la contraseña correcta debería verificar (ok=%v err=%v)", ok, err)
	}

	ok, err = VerifyPassword("incorrecta", hash)
	if err != nil {
		t.Fatalf("VerifyPassword devolvió error: %v", err)
	}
	if ok {
		t.Fatal("una contraseña incorrecta no debería verificar")
	}
}

func TestPasswordHashIsSalted(t *testing.T) {
	h1, _ := HashPassword("misma")
	h2, _ := HashPassword("misma")
	if h1 == h2 {
		t.Fatal("dos hashes de la misma contraseña deberían diferir (sal aleatoria)")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("x", "no-es-un-hash"); err == nil {
		t.Fatal("se esperaba error con hash mal formado")
	}
}
