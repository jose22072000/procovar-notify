// Package template renderiza plantillas Handlebars (raymond) y valida los
// payloads contra el JSON Schema declarado en cada template (required_variables).
package template

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/aymerick/raymond"
)

// tmplCache cachea plantillas Handlebars ya parseadas, indexadas por el hash de
// su fuente. El body/subject es inmutable por versión, así que el contenido es
// una clave estable que se autoinvalida al cambiar (equivale a cachear por
// (template, versión) sin tener que propagar el id). Evita recompilar en cada
// render — el Preview lo hacía 2× (subject + body) en cada llamada.
//
// raymond.Parse parsea de forma eager y Template.Exec solo lee el AST (sin
// mutar estado compartido), por lo que reusar una plantilla cacheada desde
// varias goroutines es seguro.
var tmplCache sync.Map // [32]byte -> *raymond.Template

// compileTemplate devuelve la plantilla parseada para source, cacheándola.
func compileTemplate(source string) (*raymond.Template, error) {
	key := sha256.Sum256([]byte(source))
	if v, ok := tmplCache.Load(key); ok {
		return v.(*raymond.Template), nil
	}
	tpl, err := raymond.Parse(source)
	if err != nil {
		return nil, err
	}
	actual, _ := tmplCache.LoadOrStore(key, tpl)
	return actual.(*raymond.Template), nil
}

// Render compila (con caché) y renderiza una plantilla Handlebars con el
// contexto dado. Se usa tanto para el cuerpo (body) como para el asunto.
func Render(source string, ctx map[string]any) (string, error) {
	tpl, err := compileTemplate(source)
	if err != nil {
		return "", fmt.Errorf("compilar handlebars: %w", err)
	}
	out, err := tpl.Exec(ctx)
	if err != nil {
		return "", fmt.Errorf("render handlebars: %w", err)
	}
	return out, nil
}
