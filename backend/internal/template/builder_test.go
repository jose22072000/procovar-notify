package template

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleStructure() Structure {
	return Structure{
		Theme: Theme{PrimaryColor: "#0B5"},
		Sections: []Section{
			{ID: "s1", Type: SectionHeader, Props: map[string]any{"title": "{{appName}}"}},
			{ID: "s2", Type: SectionText, Props: map[string]any{"text": "Hola {{firstName}},\nbienvenido."}},
			{ID: "s3", Type: SectionButton, Props: map[string]any{"text": "Activar", "url": "{{activationUrl}}"}},
			{ID: "s4", Type: SectionFooter, Props: map[string]any{"text": "© {{year}}"}},
		},
	}
}

func TestCompileForChannelPlainText(t *testing.T) {
	// Canales sin formato: cuerpo en texto plano (sin HTML), variables intactas,
	// botón como "texto: url" y sin imágenes/divisores.
	s := Structure{Sections: []Section{
		{ID: "s1", Type: SectionHeader, Props: map[string]any{"title": "Hola {{firstName}}"}},
		{ID: "s2", Type: SectionText, Props: map[string]any{"text": "Tu código es {{code}}"}},
		{ID: "s3", Type: SectionImage, Props: map[string]any{"url": "x.png"}},
		{ID: "s4", Type: SectionButton, Props: map[string]any{"text": "Abrir", "url": "{{url}}"}},
	}}
	out, err := CompileForChannel("SMS", s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(out, "<") {
		t.Errorf("el cuerpo de texto no debería contener HTML: %q", out)
	}
	for _, want := range []string{"Hola {{firstName}}", "Tu código es {{code}}", "Abrir: {{url}}"} {
		if !strings.Contains(out, want) {
			t.Errorf("el texto debería contener %q, got %q", want, out)
		}
	}
	if strings.Contains(out, "x.png") {
		t.Errorf("la imagen no debería aparecer en texto plano: %q", out)
	}
}

func TestCompileForChannelEmailIsHTML(t *testing.T) {
	out, err := CompileForChannel("EMAIL", sampleStructure())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(out, "<!doctype html>") {
		t.Error("EMAIL debería compilar a HTML")
	}
}

func TestCompileRowsColumnsAndStyles(t *testing.T) {
	// Fila con imagen (peso 1) y texto (peso 2) lado a lado, texto alineado a la
	// derecha y a 18px.
	s := Structure{Rows: []Row{{Columns: []Column{
		{Width: 1, Blocks: []Section{{ID: "a", Type: SectionImage, Props: map[string]any{"url": "a.png"}}}},
		{Width: 2, Blocks: []Section{{ID: "b", Type: SectionText, Props: map[string]any{"text": "Hola {{name}}", "align": "right", "fontSize": "18"}}}},
	}}}}
	html, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{`class="qb-col"`, `width="33%"`, `width="66%"`, "text-align:right", "font-size:18px", "a.png", "{{name}}", "@media"} {
		if !strings.Contains(html, want) {
			t.Errorf("el HTML debería contener %q", want)
		}
	}
}

func TestDeriveVariablesFromRows(t *testing.T) {
	s := Structure{Rows: []Row{{Columns: []Column{
		{Blocks: []Section{{Type: SectionText, Props: map[string]any{"text": "{{a}} y {{b}}"}}}},
	}}}}
	got := DeriveVariables(s, "{{c}}")
	for _, want := range []string{"a", "b", "c"} {
		found := false
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("debería detectar %q en %v", want, got)
		}
	}
}

func TestCompileProducesHTMLWithVarsIntact(t *testing.T) {
	html, err := Compile(sampleStructure())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{"<!doctype html>", "{{appName}}", "{{firstName}}", "{{activationUrl}}", "<br>"} {
		if !strings.Contains(html, want) {
			t.Errorf("el HTML compilado debería contener %q", want)
		}
	}
	// El botón usa el color del tema.
	if !strings.Contains(html, "#0B5") {
		t.Error("el botón debería usar el color primario del tema")
	}
}

func TestCompileInjectsWebFontLink(t *testing.T) {
	s := sampleStructure()
	s.Theme.FontImport = "https://fonts.googleapis.com/css2?family=Inter&display=swap"
	html, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// El & del URL se escapa a &amp; (correcto en un atributo HTML).
	link := `<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter&amp;display=swap">`
	if !strings.Contains(html, link) {
		t.Errorf("debería inyectar el <link> de la web font escapado; html=%s", html)
	}
	// El <link> va en el <head>, antes del <body>.
	if strings.Index(html, "<link") > strings.Index(html, "<body") {
		t.Error("el <link> debería ir en el <head>, antes del <body>")
	}
}

func TestCompileSkipsNonHTTPSFontImport(t *testing.T) {
	for _, bad := range []string{"javascript:alert(1)", "http://insecure.example/f.css", "//cdn/f.css", ""} {
		s := sampleStructure()
		s.Theme.FontImport = bad
		html, err := Compile(s)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		if strings.Contains(html, "<link") {
			t.Errorf("no debería emitir <link> para FontImport no https %q", bad)
		}
	}
}

func TestCompileEscapesLiteralText(t *testing.T) {
	s := Structure{Sections: []Section{
		{Type: SectionText, Props: map[string]any{"text": "<script>alert(1)</script>"}},
		{Type: SectionHeader, Props: map[string]any{"title": "Hola {{name}}"}},
	}}
	html, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("el texto literal <script> debería escaparse:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Errorf("texto no escapado como se esperaba:\n%s", html)
	}
	// La variable {{name}} debe conservarse (la rellena raymond en el render).
	if !strings.Contains(html, "{{name}}") {
		t.Error("la variable {{name}} debería conservarse")
	}
}

func TestCompileNeutralizesTripleStache(t *testing.T) {
	s := Structure{Sections: []Section{
		{Type: SectionText, Props: map[string]any{"text": "{{{evil}}}"}},
	}}
	html, _ := Compile(s)
	if strings.Contains(html, "{{{") {
		t.Errorf("el triple-stache debería colapsarse a {{x}}:\n%s", html)
	}
	if !strings.Contains(html, "{{evil}}") {
		t.Error("debería quedar como {{evil}} (auto-escapado en el render)")
	}
}

// TestRenderEscapesPayloadDespiteTripleStache: un autor que usa {{{x}}} (salida
// cruda) ya no puede inyectar HTML vía el payload — el valor se auto-escapa.
func TestRenderEscapesPayloadDespiteTripleStache(t *testing.T) {
	s := Structure{Sections: []Section{
		{Type: SectionText, Props: map[string]any{"text": "{{{name}}}"}},
	}}
	body, _ := Compile(s)
	out, err := Render(body, map[string]any{"name": "<img src=x onerror=alert(1)>"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(out, "<img src=x") {
		t.Errorf("el valor del payload debería auto-escaparse:\n%s", out)
	}
}

func TestDeriveVariables(t *testing.T) {
	vars := DeriveVariables(sampleStructure(), "Bienvenido a {{appName}}")
	// Orden estable (alfabético), dedup de appName.
	want := []string{"activationUrl", "appName", "firstName", "year"}
	if strings.Join(vars, ",") != strings.Join(want, ",") {
		t.Fatalf("variables derivadas inesperadas: %v", vars)
	}
}

func TestDeriveVariablesIgnoresHelpers(t *testing.T) {
	s := Structure{Sections: []Section{
		{Type: SectionText, Props: map[string]any{"text": "{{#each items}}{{name}}{{/each}}{{#if active}}x{{/if}}"}},
	}}
	vars := DeriveVariables(s, "")
	// items, name, active sí; if/each no.
	got := strings.Join(vars, ",")
	if !strings.Contains(got, "items") || !strings.Contains(got, "name") || !strings.Contains(got, "active") {
		t.Fatalf("faltan variables reales: %v", vars)
	}
	for _, kw := range []string{"if", "each"} {
		for _, v := range vars {
			if v == kw {
				t.Fatalf("no debería incluir el helper %q", kw)
			}
		}
	}
}

func TestBuildRequiredSchema(t *testing.T) {
	optional := false
	schemaJSON, err := BuildRequiredSchema(
		[]string{"firstName", "age"},
		map[string]VariableSpec{
			"age": {Type: "number", Required: &optional, Description: "Edad"},
		},
	)
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}

	var schema map[string]any
	_ = json.Unmarshal(schemaJSON, &schema)
	required, _ := schema["required"].([]any)
	if len(required) != 1 || required[0] != "firstName" {
		t.Fatalf("solo firstName debería ser requerido: %v", required)
	}
	props := schema["properties"].(map[string]any)
	age := props["age"].(map[string]any)
	if age["type"] != "number" || age["description"] != "Edad" {
		t.Fatalf("override de age no aplicado: %v", age)
	}

	// El schema generado debe ser usable por ValidatePayload.
	if err := ValidatePayload(schemaJSON, map[string]any{"firstName": "Jane"}); err != nil {
		t.Fatalf("payload válido debería pasar: %v", err)
	}
	if err := ValidatePayload(schemaJSON, map[string]any{}); err == nil {
		t.Fatal("debería fallar sin firstName")
	}
}
