package http

import (
	"net/http"
	"time"
)

// CookieConfig describe cómo se emite la cookie del refresh token. Los atributos
// dependen de la topología de despliegue (mismo dominio vs dominios distintos) y
// se configuran por entorno; ver config.CookieRefresh*.
type CookieConfig struct {
	Name     string        // p. ej. "qbn_refresh"
	Domain   string        // "" = host-only (recomendado en mismo dominio)
	Secure   bool          // true en prod/HTTPS; obligatorio con SameSite=None
	SameSite http.SameSite // Lax (mismo dominio) o None (cross-site)
}

// refreshCookiePath acota la cookie a los endpoints de auth: así el navegador la
// envía solo a /admin/auth/* (refresh y logout) y NO al resto de /admin, que se
// autentica por header Authorization. Reduce la superficie de exposición y CSRF.
const refreshCookiePath = "/admin/auth"

// refreshCookieMaxAge iguala el TTL del refresh token (7 días, ver auth.TokenIssuer).
const refreshCookieMaxAge = 7 * 24 * time.Hour

// setRefreshCookie graba la cookie HttpOnly con el refresh token.
func setRefreshCookie(w http.ResponseWriter, cfg CookieConfig, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Name,
		Value:    value,
		Path:     refreshCookiePath,
		Domain:   cfg.Domain,
		MaxAge:   int(refreshCookieMaxAge.Seconds()),
		HttpOnly: true, // inaccesible a JS: un XSS no puede robar el refresh token
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	})
}

// clearRefreshCookie borra la cookie (logout / sesión muerta).
func clearRefreshCookie(w http.ResponseWriter, cfg CookieConfig) {
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.Name,
		Value:    "",
		Path:     refreshCookiePath,
		Domain:   cfg.Domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	})
}

// readRefreshCookie devuelve el refresh token de la cookie (o "" si no está).
func readRefreshCookie(r *http.Request, cfg CookieConfig) string {
	c, err := r.Cookie(cfg.Name)
	if err != nil {
		return ""
	}
	return c.Value
}
