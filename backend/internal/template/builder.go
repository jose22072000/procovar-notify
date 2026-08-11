package template

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Section es un bloque componible del builder (§8.2). Props lleva los campos
// editables del bloque, cuyos textos admiten variables Handlebars.
type Section struct {
	ID    string         `json:"id"`
	Type  string         `json:"type"`
	Props map[string]any `json:"props"`
}

// Theme controla estilos globales del email.
type Theme struct {
	PrimaryColor string `json:"primaryColor"`
	FontFamily   string `json:"fontFamily"`
	// FontImport es la URL de una hoja de estilos de web font (p. ej. Google
	// Fonts). Si está presente y es https, se inyecta como <link> en el <head>;
	// los clientes compatibles la cargan y el resto usa la alternativa de FontFamily.
	FontImport string `json:"fontImport"`
}

// Column es una columna dentro de una fila; Width es el peso relativo (1, 2…)
// para repartir el ancho, y Blocks los bloques apilados dentro.
type Column struct {
	Width  int       `json:"width"`
	Blocks []Section `json:"blocks"`
}

// Row es una fila del cuerpo con 1..N columnas colocadas lado a lado (se apilan
// en móvil).
type Row struct {
	ID      string   `json:"id"`
	Columns []Column `json:"columns"`
}

// Structure es la definición del builder: tema + filas (cada una con columnas y
// bloques). Sections es el formato legado (lista plana), que se normaliza a una
// única fila de una columna.
type Structure struct {
	Theme    Theme     `json:"theme"`
	Rows     []Row     `json:"rows"`
	Sections []Section `json:"sections"`
}

// rows normaliza la estructura: usa Rows si existen; si no, envuelve las Sections
// legadas en una fila de una sola columna. Así las plantillas antiguas siguen
// compilando sin cambios.
func (s Structure) rows() []Row {
	if len(s.Rows) > 0 {
		return s.Rows
	}
	if len(s.Sections) > 0 {
		return []Row{{Columns: []Column{{Width: 1, Blocks: s.Sections}}}}
	}
	return nil
}

// Tipos de sección soportados en el MVP.
const (
	SectionHeader  = "header"
	SectionText    = "text"
	SectionButton  = "button"
	SectionImage   = "image"
	SectionDivider = "divider"
	SectionSpacer  = "spacer"
	SectionFooter  = "footer"
)

// ParseStructure deserializa el JSON del builder.
func ParseStructure(raw []byte) (Structure, error) {
	var s Structure
	if err := json.Unmarshal(raw, &s); err != nil {
		return Structure{}, fmt.Errorf("structure inválida: %w", err)
	}
	return s, nil
}

// defaults del tema.
func (t Theme) withDefaults() Theme {
	if t.PrimaryColor == "" {
		t.PrimaryColor = "#0B5"
	}
	if t.FontFamily == "" {
		t.FontFamily = "Arial, Helvetica, sans-serif"
	}
	return t
}

// CompileForChannel compila la estructura al cuerpo adecuado para el canal:
// EMAIL produce HTML responsive; SMS/PUSH/IN_APP producen texto plano (título y
// cuerpo van luego como subject + body). Las variables {{x}} se conservan; el
// render con el payload ocurre en el worker.
func CompileForChannel(channel string, s Structure) (string, error) {
	if channel == "" || channel == "EMAIL" {
		return Compile(s)
	}
	return compilePlainText(s), nil
}

// CompileForChannelPreview es como CompileForChannel pero en modo previsualización
// (marcadores para imágenes sin URL). Se usa en el editor en vivo, no al persistir.
func CompileForChannelPreview(channel string, s Structure) (string, error) {
	if channel == "" || channel == "EMAIL" {
		return CompilePreview(s)
	}
	return compilePlainText(s), nil
}

// compilePlainText extrae el texto de las secciones para los canales sin formato
// (SMS, cuerpo de PUSH/IN_APP). Recorre las props de texto conocidas y las une
// con líneas en blanco, preservando las variables Handlebars.
func compilePlainText(s Structure) string {
	var lines []string
	add := func(v string) {
		if strings.TrimSpace(v) != "" {
			lines = append(lines, v)
		}
	}
	for _, r := range s.rows() {
		for _, c := range r.Columns {
			for _, sec := range c.Blocks {
				switch sec.Type {
				case SectionDivider, SectionSpacer, SectionImage:
					// Sin texto relevante en canales de solo texto.
				case SectionButton:
					txt, url := propStr(sec.Props, "text"), propStr(sec.Props, "url")
					switch {
					case txt != "" && url != "":
						add(txt + ": " + url)
					case url != "":
						add(url)
					default:
						add(txt)
					}
				default: // header/title/text/body/footer y tipos propios de otros canales
					add(propStr(sec.Props, "title"))
					add(propStr(sec.Props, "text"))
					add(propStr(sec.Props, "body"))
				}
			}
		}
	}
	return strings.Join(lines, "\n\n")
}

// Compile transforma la estructura del builder en un cuerpo HTML de email
// responsive con estilos inline (compatibilidad amplia con clientes de correo).
// Los textos conservan sus variables Handlebars ({{x}}): el render final con el
// payload ocurre en el worker.
//
// La estructura ya se valida al parsearla (ParseStructure), así que hoy Compile
// no produce errores; conserva el retorno de error por robustez ante futuras
// validaciones (el caller mapea cualquier error a invalid_structure).
func Compile(s Structure) (string, error) {
	return compileHTML(s, false), nil
}

// CompilePreview compila igual que Compile pero para previsualización: los bloques
// de imagen sin URL muestran un marcador ("aquí va una imagen") en vez de omitirse.
func CompilePreview(s Structure) (string, error) {
	return compileHTML(s, true), nil
}

func compileHTML(s Structure, preview bool) string {
	theme := s.Theme.withDefaults()
	var b strings.Builder

	b.WriteString(`<!doctype html><html><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	if u := theme.FontImport; strings.HasPrefix(u, "https://") {
		fmt.Fprintf(&b, `<link rel="stylesheet" href="%s">`, esc(u))
	}
	// Apilado responsive: en móvil cada columna pasa a ocupar el 100% del ancho.
	b.WriteString(`<style>@media only screen and (max-width:600px){.qb-col{display:block!important;width:100%!important}}</style>`)
	b.WriteString(`</head>`)
	fmt.Fprintf(&b, `<body style="margin:0;padding:0;background:#f4f4f4;font-family:%s;">`, esc(theme.FontFamily))
	b.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f4f4;"><tr><td align="center" style="padding:24px;">`)
	b.WriteString(`<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border-radius:8px;overflow:hidden;">`)

	for _, r := range s.rows() {
		b.WriteString(renderRow(r, theme, preview))
	}

	b.WriteString(`</table></td></tr></table></body></html>`)
	return b.String()
}

// renderRow produce una fila (<tr>) con una tabla interna de columnas (<td>) que
// se colocan lado a lado y se apilan en móvil (clase qb-col + media query).
func renderRow(r Row, theme Theme, preview bool) string {
	cols := r.Columns
	if len(cols) == 0 {
		return ""
	}
	total := 0
	for _, c := range cols {
		total += colWidth(c)
	}
	var b strings.Builder
	b.WriteString(`<tr><td style="padding:0;"><table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr>`)
	for _, c := range cols {
		pct := 100 * colWidth(c) / total
		fmt.Fprintf(&b, `<td class="qb-col" valign="top" width="%d%%" style="width:%d%%;">`, pct, pct)
		for _, blk := range c.Blocks {
			b.WriteString(renderBlock(blk, theme, preview))
		}
		b.WriteString(`</td>`)
	}
	b.WriteString(`</tr></table></td></tr>`)
	return b.String()
}

func colWidth(c Column) int {
	if c.Width <= 0 {
		return 1
	}
	return c.Width
}

// renderBlock produce el HTML interno de un bloque (sin envoltura de fila): se
// coloca dentro del <td> de su columna. Aplica alineación, tamaño y color según
// las props de estilo del bloque.
func renderBlock(sec Section, theme Theme, preview bool) string {
	p := func(k string) string { return propStr(sec.Props, k) }
	switch sec.Type {
	case SectionHeader:
		inner := ""
		if logo := p("logoUrl"); logo != "" {
			inner += fmt.Sprintf(`<img src="%s" alt="" height="40" style="display:inline-block;margin:0 0 12px;">`, esc(logo))
		}
		inner += fmt.Sprintf(`<h1 style="margin:0;font-size:%dpx;color:%s;">%s</h1>`, fontSizeOr(p("fontSize"), 22), esc(colorOr(p("color"), "#111111")), esc(p("title")))
		return fmt.Sprintf(`<div style="padding:24px 24px 8px;text-align:%s;">%s</div>`, alignOr(p("align"), "center"), inner)
	case SectionText:
		return fmt.Sprintf(`<div style="padding:8px 24px;font-size:%dpx;line-height:1.5;color:%s;text-align:%s;">%s</div>`,
			fontSizeOr(p("fontSize"), 15), esc(colorOr(p("color"), "#333333")), alignOr(p("align"), "left"), nl2br(esc(p("text"))))
	case SectionButton:
		return fmt.Sprintf(`<div style="padding:16px 24px;text-align:%s;"><a href="%s" style="display:inline-block;padding:12px 22px;background:%s;color:#fff;text-decoration:none;border-radius:6px;font-size:15px;">%s</a></div>`,
			alignOr(p("align"), "center"), esc(p("url")), esc(theme.PrimaryColor), esc(p("text")))
	case SectionImage:
		if p("url") == "" {
			// Sin URL: en preview mostramos un marcador; en el email real se omite
			// (para no enviar una imagen rota).
			if !preview {
				return ""
			}
			return `<div style="padding:8px 24px;text-align:center;"><div style="border:2px dashed #d4d4d4;background:#fafafa;border-radius:8px;padding:24px 12px;color:#9ca3af;font-size:13px;font-family:Arial,Helvetica,sans-serif;">` +
				`<svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="#9ca3af" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2" ry="2"></rect><circle cx="9" cy="9" r="2"></circle><path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"></path></svg>` +
				`<div style="margin-top:8px;">Aquí va una imagen · añade una URL</div></div></div>`
		}
		return fmt.Sprintf(`<div style="padding:8px 24px;text-align:%s;"><img src="%s" alt="%s" style="display:inline-block;max-width:100%%;height:auto;"></div>`,
			alignOr(p("align"), "center"), esc(p("url")), esc(p("alt")))
	case SectionDivider:
		if p("orientation") == "vertical" {
			return fmt.Sprintf(`<div style="padding:8px 24px;text-align:%s;"><div style="display:inline-block;width:%dpx;height:%dpx;background:#e5e5e5;"></div></div>`,
				alignOr(p("align"), "center"), pxOr(p("thickness"), 1, 1, 40), pxOr(p("height"), 40, 4, 600))
		}
		return fmt.Sprintf(`<div style="padding:8px 24px;"><hr style="border:none;border-top:%dpx solid #e5e5e5;margin:0;"></div>`, pxOr(p("thickness"), 1, 1, 40))
	case SectionSpacer:
		h := p("height")
		if h == "" {
			h = "16"
		}
		return fmt.Sprintf(`<div style="font-size:0;line-height:0;height:%spx;">&nbsp;</div>`, esc(h))
	case SectionFooter:
		return fmt.Sprintf(`<div style="padding:16px 24px 24px;text-align:%s;font-size:%dpx;color:%s;">%s</div>`,
			alignOr(p("align"), "center"), fontSizeOr(p("fontSize"), 12), esc(colorOr(p("color"), "#999999")), nl2br(esc(p("text"))))
	default:
		return ""
	}
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

// alignOr valida la alineación (whitelist) y devuelve def si no es válida.
func alignOr(a, def string) string {
	switch a {
	case "left", "center", "right", "justify":
		return a
	}
	return def
}

// fontSizeOr parsea un tamaño en px acotado a [8,96]; def si no es válido.
func fontSizeOr(s string, def int) int {
	return pxOr(s, def, 8, 96)
}

// pxOr parsea un entero en px acotado a [min,max]; def si no es válido.
func pxOr(s string, def, min, max int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < min || n > max {
		return def
	}
	return n
}

// colorOr devuelve el color si es un hex válido; def en caso contrario.
func colorOr(s, def string) string {
	if hexColor.MatchString(strings.TrimSpace(s)) {
		return strings.TrimSpace(s)
	}
	return def
}

func propStr(props map[string]any, key string) string {
	if props == nil {
		return ""
	}
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// esc escapa el texto/atributo literal del autor (HTML: <>&'") y neutraliza las
// salidas SIN escapar de handlebars ({{{x}}} triple-stache y {{&x}}) colapsándolas
// a {{x}}. Preserva {{x}}, que raymond rellena y auto-escapa en el render final
// con el payload. Evita XSS almacenado tanto vía texto literal como vía variables.
func esc(s string) string {
	return neutralizeTripleStache(html.EscapeString(s))
}

// neutralizeTripleStache colapsa el «triple-stache» {{{x}}} → {{x}} (y }}} → }})
// para que raymond NO emita HTML sin escapar. Es la defensa clave del modo HTML:
// el saneador limpia el HTML del autor una vez, pero {{{payload}}} inyectaría HTML
// del payload sin sanear en cada render; colapsarlo fuerza el auto-escape.
func neutralizeTripleStache(s string) string {
	s = strings.ReplaceAll(s, "{{{", "{{")
	s = strings.ReplaceAll(s, "}}}", "}}")
	return s
}

// nl2br convierte saltos de línea en <br> (los textos del builder son planos).
func nl2br(s string) string { return strings.ReplaceAll(s, "\n", "<br>") }

// --- Derivación de variables ---

// mustacheVar captura: [1]=prefijo (#,^,/ o vacío), [2]=primer identificador
// (helper o variable), [3]=segundo identificador opcional (la variable cuando
// [2] es un helper de bloque, p. ej. {{#each items}} → items).
var mustacheVar = regexp.MustCompile(`\{\{[~]?\s*([#^/]?)\s*([a-zA-Z_][a-zA-Z0-9_]*)(?:\s+([a-zA-Z_][a-zA-Z0-9_.]*))?`)

// helperKeywords son palabras de Handlebars que no son variables.
var helperKeywords = map[string]bool{
	"if": true, "unless": true, "each": true, "with": true, "else": true, "lookup": true,
}

// root devuelve el segmento raíz de una ruta con puntos (user.name → user).
func root(ident string) string {
	if i := strings.IndexByte(ident, '.'); i >= 0 {
		return ident[:i]
	}
	return ident
}

// scanVars acumula en set las variables {{...}} encontradas en un texto.
func scanVars(set map[string]bool, text string) {
	for _, m := range mustacheVar.FindAllStringSubmatch(text, -1) {
		prefix, first, second := m[1], m[2], m[3]
		if prefix == "/" {
			continue // cierre de bloque, p. ej. {{/each}}
		}
		if helperKeywords[first] {
			// Helper de bloque: la variable es el argumento (si lo hay).
			if second != "" && !helperKeywords[root(second)] {
				set[root(second)] = true
			}
			continue
		}
		set[root(first)] = true
	}
}

// sortedKeys devuelve las claves del set ordenadas (orden estable).
func sortedKeys(set map[string]bool) []string {
	vars := make([]string, 0, len(set))
	for v := range set {
		vars = append(vars, v)
	}
	sort.Strings(vars)
	return vars
}

// DeriveVariables extrae los nombres de variable usados en el subject y en todas
// las props de texto de las secciones (dedup, orden estable).
func DeriveVariables(s Structure, subject string) []string {
	set := map[string]bool{}
	scanVars(set, subject)
	for _, r := range s.rows() {
		for _, c := range r.Columns {
			for _, sec := range c.Blocks {
				for _, v := range sec.Props {
					if str, ok := v.(string); ok {
						scanVars(set, str)
					}
				}
			}
		}
	}
	return sortedKeys(set)
}

// DeriveVariablesFromText extrae variables {{...}} de textos planos (subject + HTML
// crudo del modo avanzado), con la misma semántica que DeriveVariables.
func DeriveVariablesFromText(texts ...string) []string {
	set := map[string]bool{}
	for _, t := range texts {
		scanVars(set, t)
	}
	return sortedKeys(set)
}

// VariableSpec permite al admin afinar una variable derivada.
type VariableSpec struct {
	Required    *bool  `json:"required,omitempty"`
	Type        string `json:"type,omitempty"` // string|number|boolean (default string)
	Description string `json:"description,omitempty"`
}

// BuildRequiredSchema construye el JSON Schema (required_variables) a partir de
// las variables derivadas y los overrides del admin. Por defecto cada variable
// es string y requerida.
func BuildRequiredSchema(vars []string, overrides map[string]VariableSpec) ([]byte, error) {
	props := map[string]any{}
	var required []string
	for _, v := range vars {
		typ := "string"
		isRequired := true
		prop := map[string]any{}
		if ov, ok := overrides[v]; ok {
			if ov.Type != "" {
				typ = ov.Type
			}
			if ov.Required != nil {
				isRequired = *ov.Required
			}
			if ov.Description != "" {
				prop["description"] = ov.Description
			}
		}
		prop["type"] = typ
		props[v] = prop
		if isRequired {
			required = append(required, v)
		}
	}
	sort.Strings(required)
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return json.Marshal(schema)
}
