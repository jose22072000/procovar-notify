package audit

import (
	"context"
	"testing"
)

func TestRequestMetaContext(t *testing.T) {
	// Sin meta → cadenas vacías.
	if ip, ua := RequestMetaFromContext(context.Background()); ip != "" || ua != "" {
		t.Errorf("sin meta debería devolver vacíos, got ip=%q ua=%q", ip, ua)
	}
	// Roundtrip.
	ctx := ContextWithRequestMeta(context.Background(), "203.0.113.7", "curl/8.0")
	ip, ua := RequestMetaFromContext(ctx)
	if ip != "203.0.113.7" || ua != "curl/8.0" {
		t.Errorf("meta mal recuperada: ip=%q ua=%q", ip, ua)
	}
}

func TestStrPtr(t *testing.T) {
	if strPtr("") != nil {
		t.Error("strPtr(\"\") debería ser nil (NULL en BD)")
	}
	if p := strPtr("x"); p == nil || *p != "x" {
		t.Error("strPtr(\"x\") debería apuntar a \"x\"")
	}
}
