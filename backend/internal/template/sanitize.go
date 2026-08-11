package template

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Saneamiento del HTML de las plantillas en modo avanzado. Los correos son
// documentos completos con <style>, así que NO usamos un saneador de fragmentos
// (que se comería <head>/<style>): parseamos el árbol con x/net/html y aplicamos
// una lista blanca, conservando estructura, <style> y estilos inline pero
// eliminando todo lo ejecutable (scripts, manejadores on*, esquemas peligrosos).
//
// Es defensa del PANEL de admin (el preview corre en su navegador): los clientes
// de correo ya quitan el JS del email. Se aplica al guardar y al previsualizar.

// MaxEmailHTMLBytes limita el tamaño del HTML crudo aceptado.
const MaxEmailHTMLBytes = 256 * 1024

// Etiquetas que se eliminan por completo (con su contenido). Incluye svg/math:
// son contenido "foreign" (la principal superficie de mXSS) y no tienen uso en
// email (los clientes los eliminan de todos modos).
var blockedTags = map[string]bool{
	"script": true, "iframe": true, "object": true, "embed": true,
	"link": true, "base": true, "form": true, "applet": true,
	"frame": true, "frameset": true, "svg": true, "math": true,
}

// Atributos de URL cuyo esquema hay que validar.
var urlAttrs = map[string]bool{
	"href": true, "src": true, "xlink:href": true, "background": true,
	"action": true, "formaction": true, "poster": true, "data": true,
}

var (
	dangerousScheme = regexp.MustCompile(`(?i)^\s*(javascript|vbscript):`)
	schemeAnywhere  = regexp.MustCompile(`(?i)(javascript|vbscript):`)
	dataScheme      = regexp.MustCompile(`(?i)^\s*data:`)
	dataImage       = regexp.MustCompile(`(?i)^\s*data:image/`)
	cssExpression   = regexp.MustCompile(`(?i)expression\s*\(`)
	cssImport       = regexp.MustCompile(`(?i)@import[^;]*;?`)
	cssURLScheme    = regexp.MustCompile(`(?i)(javascript|vbscript)\s*:`)
	cssBinding      = regexp.MustCompile(`(?i)(behavior|-moz-binding)\s*:[^;]*;?`)
)

// SanitizeEmailHTML valida el tamaño, parsea y devuelve el HTML saneado.
func SanitizeEmailHTML(input string) (string, error) {
	if len(input) > MaxEmailHTMLBytes {
		return "", fmt.Errorf("el HTML supera el máximo de %d KB", MaxEmailHTMLBytes/1024)
	}
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("el HTML está vacío")
	}
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		return "", fmt.Errorf("HTML no válido: %w", err)
	}
	sanitizeNode(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("no se pudo serializar el HTML: %w", err)
	}
	return buf.String(), nil
}

// sanitizeNode recorre el árbol eliminando nodos/atributos peligrosos.
func sanitizeNode(n *html.Node) {
	var next *html.Node
	for c := n.FirstChild; c != nil; c = next {
		next = c.NextSibling
		if c.Type == html.CommentNode {
			n.RemoveChild(c) // los comentarios (p. ej. condicionales de IE) no aportan y sí riesgo
			continue
		}
		if c.Type == html.ElementNode {
			tag := strings.ToLower(c.Data)
			if blockedTags[tag] || isRefreshMeta(c) {
				n.RemoveChild(c)
				continue
			}
			cleanAttrs(c)
			if tag == "style" {
				sanitizeStyleText(c) // el CSS es texto crudo dentro del <style>
				continue
			}
		}
		sanitizeNode(c)
	}
}

// cleanAttrs elimina manejadores on* y neutraliza URLs/estilos peligrosos.
func cleanAttrs(n *html.Node) {
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		switch {
		case strings.HasPrefix(key, "on"): // onclick, onerror, onload…
			continue
		case key == "srcdoc": // nunca permitir documentos anidados
			continue
		case urlAttrs[key]:
			if isDangerousURL(a.Val) {
				continue
			}
		case key == "srcset":
			// srcset es multi-valor ("url 1x, url 2x"); descarta el atributo entero
			// si aparece un esquema peligroso en cualquier candidato.
			if schemeAnywhere.MatchString(a.Val) {
				continue
			}
		case key == "style":
			a.Val = sanitizeCSS(a.Val)
		}
		kept = append(kept, a)
	}
	n.Attr = kept
}

// isDangerousURL bloquea javascript:/vbscript: y data: (salvo data:image/*).
func isDangerousURL(v string) bool {
	if dangerousScheme.MatchString(v) {
		return true
	}
	if dataScheme.MatchString(v) && !dataImage.MatchString(v) {
		return true
	}
	return false
}

// sanitizeCSS neutraliza construcciones peligrosas en CSS (inline o en <style>).
func sanitizeCSS(s string) string {
	s = cssImport.ReplaceAllString(s, "")
	s = cssExpression.ReplaceAllString(s, "blocked(")
	s = cssBinding.ReplaceAllString(s, "")
	s = cssURLScheme.ReplaceAllString(s, "blocked:")
	return s
}

// sanitizeStyleText limpia el CSS del contenido de un elemento <style>.
func sanitizeStyleText(style *html.Node) {
	for c := style.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			c.Data = sanitizeCSS(c.Data)
		}
	}
}

// isRefreshMeta detecta <meta http-equiv="refresh"> (redirección).
func isRefreshMeta(n *html.Node) bool {
	if strings.ToLower(n.Data) != "meta" {
		return false
	}
	for _, a := range n.Attr {
		if strings.ToLower(a.Key) == "http-equiv" && strings.EqualFold(strings.TrimSpace(a.Val), "refresh") {
			return true
		}
	}
	return false
}
