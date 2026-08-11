package template

import "testing"

func TestRender(t *testing.T) {
	out, err := Render("Hola {{firstName}}, bienvenido a {{appName}}", map[string]any{
		"firstName": "Jane", "appName": "Acme",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if out != "Hola Jane, bienvenido a Acme" {
		t.Fatalf("render inesperado: %q", out)
	}
}

func TestValidatePayload(t *testing.T) {
	schema := []byte(`{"type":"object","required":["firstName","activationUrl"],` +
		`"properties":{"firstName":{"type":"string"},"activationUrl":{"type":"string"}}}`)

	t.Run("payload válido", func(t *testing.T) {
		err := ValidatePayload(schema, map[string]any{
			"firstName": "Jane", "activationUrl": "https://x",
		})
		if err != nil {
			t.Fatalf("debería ser válido: %v", err)
		}
	})

	t.Run("falta variable requerida", func(t *testing.T) {
		err := ValidatePayload(schema, map[string]any{"firstName": "Jane"})
		if err == nil {
			t.Fatal("debería fallar por activationUrl faltante")
		}
	})

	t.Run("tipo incorrecto", func(t *testing.T) {
		err := ValidatePayload(schema, map[string]any{"firstName": 123, "activationUrl": "x"})
		if err == nil {
			t.Fatal("debería fallar por tipo de firstName")
		}
	})

	t.Run("schema vacío no valida", func(t *testing.T) {
		if err := ValidatePayload(nil, map[string]any{"x": 1}); err != nil {
			t.Fatalf("schema vacío no debería fallar: %v", err)
		}
	})
}

// TestCompileCaches verifica M10: misma fuente reutiliza la compilación
// (mismo puntero) y fuentes distintas compilan por separado.
func TestCompileCaches(t *testing.T) {
	t.Run("template", func(t *testing.T) {
		src := "Hola {{name}} #" + t.Name()
		a, err := compileTemplate(src)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		b, err := compileTemplate(src)
		if err != nil {
			t.Fatalf("compile 2: %v", err)
		}
		if a != b {
			t.Fatal("la misma fuente debería devolver la plantilla cacheada (mismo puntero)")
		}
		if c, _ := compileTemplate(src + " v2"); c == a {
			t.Fatal("una fuente distinta no debería compartir caché")
		}
		// Fuente inválida: error y no se cachea como entrada válida.
		if _, err := compileTemplate("{{#if}}sin cierre"); err == nil {
			t.Fatal("fuente inválida debería dar error")
		}
	})

	t.Run("schema", func(t *testing.T) {
		schema := []byte(`{"type":"object","properties":{"a":{"type":"string"}}}`)
		a, err := compileSchema(schema)
		if err != nil {
			t.Fatalf("compile schema: %v", err)
		}
		b, err := compileSchema(schema)
		if err != nil {
			t.Fatalf("compile schema 2: %v", err)
		}
		if a != b {
			t.Fatal("el mismo schema debería devolver el compilado cacheado (mismo puntero)")
		}
		if _, err := compileSchema([]byte("{not json")); err == nil {
			t.Fatal("schema inválido debería dar error")
		}
	})
}
