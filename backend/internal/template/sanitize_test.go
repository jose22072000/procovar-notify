package template

import (
	"strings"
	"testing"
)

func mustSanitize(t *testing.T, in string) string {
	t.Helper()
	out, err := SanitizeEmailHTML(in)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	return out
}

func TestSanitizeRemovesScript(t *testing.T) {
	out := mustSanitize(t, `<div>ok<script>alert('x')</script></div>`)
	if strings.Contains(out, "<script") || strings.Contains(out, "alert(") {
		t.Fatalf("no debería quedar script ni su contenido: %q", out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("debería conservar el texto legítimo: %q", out)
	}
}

func TestSanitizeRemovesEventHandlers(t *testing.T) {
	out := mustSanitize(t, `<img src="logo.png" onerror="steal()" onload="x()">`)
	if strings.Contains(out, "onerror") || strings.Contains(out, "onload") || strings.Contains(out, "steal(") {
		t.Fatalf("no debería quedar ningún on*: %q", out)
	}
	if !strings.Contains(out, `src="logo.png"`) {
		t.Fatalf("debería conservar el src legítimo: %q", out)
	}
}

func TestSanitizeNeutralizesJavascriptURL(t *testing.T) {
	out := mustSanitize(t, `<a href="javascript:alert(1)">clic</a>`)
	if strings.Contains(out, "javascript:") {
		t.Fatalf("no debería quedar el esquema javascript: %q", out)
	}
	if !strings.Contains(out, "clic") {
		t.Fatalf("debería conservar el texto del enlace: %q", out)
	}
}

func TestSanitizeAllowsSafeURLsAndDataImages(t *testing.T) {
	out := mustSanitize(t, `<a href="https://ok.com"><img src="data:image/png;base64,AAAA"></a>`)
	if !strings.Contains(out, "https://ok.com") {
		t.Fatalf("debería conservar https: %q", out)
	}
	if !strings.Contains(out, "data:image/png") {
		t.Fatalf("debería conservar data:image: %q", out)
	}
}

func TestSanitizeRemovesNonImageDataURL(t *testing.T) {
	out := mustSanitize(t, `<a href="data:text/html,<b>x</b>">y</a>`)
	if strings.Contains(out, "data:text/html") {
		t.Fatalf("no debería quedar data:text/html: %q", out)
	}
}

func TestSanitizeKeepsStyleButCleansCSS(t *testing.T) {
	in := `<html><head><style>
	  .btn { color: #fff; background: expression(alert(1)); }
	  @import url('http://evil/x.css');
	  .b { behavior: url(#default#x); }
	</style></head><body><p style="width:expression(alert(2))">hola</p></body></html>`
	out := mustSanitize(t, in)
	if !strings.Contains(out, "<style") {
		t.Fatalf("debería conservar el bloque <style>: %q", out)
	}
	if !strings.Contains(out, "color: #fff") {
		t.Fatalf("debería conservar el CSS legítimo: %q", out)
	}
	for _, bad := range []string{"expression(", "@import", "behavior:"} {
		if strings.Contains(out, bad) {
			t.Fatalf("el CSS peligroso %q no debería quedar: %q", bad, out)
		}
	}
}

func TestSanitizeRemovesDangerousElements(t *testing.T) {
	in := `<div>
	  <iframe src="https://e"></iframe>
	  <object data="x"></object>
	  <embed src="x">
	  <link rel="stylesheet" href="http://e/x.css">
	  <base href="http://e/">
	  <form action="http://e"><input></form>
	  <meta http-equiv="refresh" content="0;url=http://e">
	  <p>contenido</p>
	</div>`
	out := mustSanitize(t, in)
	for _, bad := range []string{"<iframe", "<object", "<embed", "<link", "<base", "<form", "http-equiv"} {
		if strings.Contains(out, bad) {
			t.Fatalf("no debería quedar %q: %q", bad, out)
		}
	}
	if !strings.Contains(out, "contenido") {
		t.Fatalf("debería conservar el contenido legítimo: %q", out)
	}
}

func TestSanitizeKeepsVariablesAndTables(t *testing.T) {
	in := `<table><tr><td>Hola {{firstName}}, tu código es {{otpCode}}</td></tr></table>`
	out := mustSanitize(t, in)
	for _, keep := range []string{"<table", "<td", "{{firstName}}", "{{otpCode}}"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("debería conservar %q: %q", keep, out)
		}
	}
}

func TestSanitizeRemovesForeignContentAndComments(t *testing.T) {
	in := `<div>ok<svg><script>alert(1)</script></svg><math><mtext>x</mtext></math><!--[if IE]><script>alert(2)</script><![endif]--></div>`
	out := mustSanitize(t, in)
	for _, bad := range []string{"<svg", "<math", "<script", "if IE", "alert("} {
		if strings.Contains(out, bad) {
			t.Fatalf("no debería quedar %q: %q", bad, out)
		}
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("debería conservar el texto legítimo: %q", out)
	}
}

func TestSanitizeValidatesSrcset(t *testing.T) {
	out := mustSanitize(t, `<img src="a.png" srcset="a.png 1x, javascript:alert(1) 2x">`)
	if strings.Contains(out, "javascript:") {
		t.Fatalf("srcset con javascript: debería eliminarse: %q", out)
	}
}

func TestSanitizeRejectsEmptyAndOversize(t *testing.T) {
	if _, err := SanitizeEmailHTML("   "); err == nil {
		t.Fatal("HTML vacío debería dar error")
	}
	big := strings.Repeat("a", MaxEmailHTMLBytes+1)
	if _, err := SanitizeEmailHTML(big); err == nil {
		t.Fatal("HTML sobredimensionado debería dar error")
	}
}

func TestDeriveVariablesFromText(t *testing.T) {
	vars := DeriveVariablesFromText(
		"Hola {{firstName}}",
		`<p>Tu código {{otpCode}} caduca. {{#if url}}<a href="{{url}}">ir</a>{{/if}}</p>`,
	)
	got := strings.Join(vars, ",")
	for _, want := range []string{"firstName", "otpCode", "url"} {
		if !strings.Contains(got, want) {
			t.Fatalf("debería incluir %q: %v", want, vars)
		}
	}
	// orden estable + sin helpers (if/each).
	if strings.Contains(got, "if") {
		t.Fatalf("no debería incluir el helper 'if': %v", vars)
	}
}
