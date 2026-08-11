package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeEncryptionKey(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString(make([]byte, 32))

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "válida 32 bytes", raw: valid, wantErr: false},
		{name: "vacía", raw: "", wantErr: true},
		{name: "no base64", raw: "no-es-base64-!!!", wantErr: true},
		{name: "longitud incorrecta", raw: base64.StdEncoding.EncodeToString(make([]byte, 16)), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := decodeEncryptionKey(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("se esperaba error y no hubo")
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if len(key) != 32 {
				t.Fatalf("clave de %d bytes, se esperaban 32", len(key))
			}
		})
	}
}

func TestValidate(t *testing.T) {
	base := func() *Config {
		return &Config{
			DatabaseURL:    "postgres://localhost/db",
			AdminJWTSecret: strings.Repeat("a", 32),
			APIPort:        8080,
			Redis:          RedisConfig{Addr: "localhost:6379"},
		}
	}

	t.Run("config mínima válida", func(t *testing.T) {
		if err := base().validate(); err != nil {
			t.Fatalf("no debería fallar: %v", err)
		}
	})

	t.Run("acumula múltiples problemas", func(t *testing.T) {
		c := &Config{APIPort: 0} // sin DB, sin JWT, sin redis, puerto inválido
		err := c.validate()
		if err == nil {
			t.Fatal("se esperaba error")
		}
		for _, want := range []string{"DATABASE_URL", "ADMIN_JWT_SECRET", "REDIS", "API_PORT"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("el error debería mencionar %q; fue: %v", want, err)
			}
		}
	})

	t.Run("ADMIN_JWT_SECRET corto falla", func(t *testing.T) {
		c := base()
		c.AdminJWTSecret = "demasiado-corto" // < 32
		if err := c.validate(); err == nil {
			t.Fatal("se esperaba error por ADMIN_JWT_SECRET < 32 bytes")
		}
	})

	t.Run("sentinel sin master name falla", func(t *testing.T) {
		c := base()
		c.Redis = RedisConfig{Sentinels: []string{"h:26379"}}
		if err := c.validate(); err == nil {
			t.Fatal("se esperaba error por falta de MASTER_NAME")
		}
	})
}

func TestRedisUseSentinel(t *testing.T) {
	if (RedisConfig{}).UseSentinel() {
		t.Error("sin sentinels no debería usar sentinel")
	}
	if !(RedisConfig{Sentinels: []string{"h:26379"}}).UseSentinel() {
		t.Error("con sentinels debería usar sentinel")
	}
}
