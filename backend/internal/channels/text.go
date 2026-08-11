package channels

import (
	"regexp"
	"strings"
)

var (
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe = regexp.MustCompile(`[ \t]+`)
	blankRe = regexp.MustCompile(`\n{3,}`)
)

// htmlToText convierte un cuerpo HTML en texto plano razonable para canales sin
// formato (SMS, cuerpo de la notificación push). No pretende ser un renderizador
// completo: elimina etiquetas, decodifica las entidades más comunes y normaliza
// los espacios en blanco.
func htmlToText(html string) string {
	s := html
	// Saltos de línea a partir de bloques habituales.
	s = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`(?i)</(p|div|tr|h[1-6]|li)>`).ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	replacer := strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'",
	)
	s = replacer.Replace(s)
	s = spaceRe.ReplaceAllString(s, " ")
	s = blankRe.ReplaceAllString(s, "\n\n")
	// Recorta espacios sobrantes por línea.
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(ln)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// messageText elige el mejor texto plano disponible para el mensaje: el cuerpo
// renderizado a texto y, en su defecto, el asunto.
func messageText(msg RenderedMessage) string {
	if t := htmlToText(msg.HTMLBody); t != "" {
		return t
	}
	return msg.Subject
}
