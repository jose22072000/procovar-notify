package http

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"dvtech/qbn/internal/auth"
)

// Entrada por la sesion unica de Procovar.
//
// Aceptar la cookie del hub (ver auth.AdminAuthenticator.desdeProcovar) resuelve
// el caso de quien YA entro en Procovar. Esto resuelve el otro: mandarla a
// entrar, como hacen las demas aplicaciones.
//
// Mismo flujo que procovar-rutas, que es la implementacion de referencia:
//
//	/admin/auth/sso/login    -> pide un callback token y redirige al hub
//	/admin/auth/sso/callback -> vuelve con ?code=…, se canjea y se graba la cookie

// ssoCookieName es la cookie PROPIA de Avisos con el token de sesion del hub.
//
// Se graba aparte de la compartida (`__Secure-qb.session_token`) a proposito: la
// compartida solo llega si el navegador la manda —depende de que el hub y Avisos
// esten bajo el mismo dominio raiz—, y esta la controlamos nosotros. El
// middleware acepta cualquiera de las dos, asi que el dia que cambie la
// topologia de dominios esto sigue funcionando.
// Vive en internal/auth para que el middleware la lea sin importar este paquete.
const ssoCookieName = auth.CookieSSOPropia

type SSOHandler struct {
	hub    *auth.ProcovarAuth
	appURL string // https://avisos.procovar.cloud
	apiURL string // https://avisos-api.procovar.cloud
	secure bool
}

func NewSSOHandler(hub *auth.ProcovarAuth, appURL, apiURL string, secure bool) *SSOHandler {
	return &SSOHandler{hub: hub, appURL: strings.TrimRight(appURL, "/"), apiURL: strings.TrimRight(apiURL, "/"), secure: secure}
}

// Disponible indica si el SSO esta configurado. Sin hub no se montan las rutas.
func (h *SSOHandler) Disponible() bool { return h != nil && h.hub != nil }

func (h *SSOHandler) Login(w http.ResponseWriter, r *http.Request) {
	volverA := h.destinoSeguro(r.URL.Query().Get("returnTo"))
	destino, err := h.hub.CrearTokenDeVuelta(r.Context(), h.apiURL+"/admin/auth/sso/callback", volverA)
	if err != nil {
		// Si el hub no contesta se vuelve al login propio en vez de dejar una
		// pantalla en blanco: Avisos tiene que poder administrarse justo cuando
		// algo va mal.
		http.Redirect(w, r, h.appURL+"/login?sso=no-disponible", http.StatusFound)
		return
	}
	http.Redirect(w, r, destino, http.StatusFound)
}

func (h *SSOHandler) Callback(w http.ResponseWriter, r *http.Request) {
	codigo := r.URL.Query().Get("code")
	if codigo == "" {
		http.Redirect(w, r, h.appURL+"/login?sso=sin-codigo", http.StatusFound)
		return
	}
	token, volverA, err := h.hub.Canjear(r.Context(), codigo)
	if err != nil {
		http.Redirect(w, r, h.appURL+"/login?sso=canje-fallido", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     ssoCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   h.secure,
		// None porque la pantalla (avisos.) y la API (avisos-api.) son origenes
		// distintos: con Lax el navegador no la mandaria.
		SameSite: http.SameSiteNoneMode,
	})
	http.Redirect(w, r, h.destinoSeguro(volverA), http.StatusFound)
}

// destinoSeguro impide que ?returnTo= mande a nadie fuera de Avisos.
//
// Sin esto, un enlace preparado con returnTo=https://sitio-malo/ convierte el
// login en un trampolin: la persona entra de verdad y acaba en otro sitio
// creyendo que sigue en casa.
func (h *SSOHandler) destinoSeguro(destino string) string {
	if destino == "" {
		return h.appURL
	}
	u, err := url.Parse(destino)
	if err != nil {
		return h.appURL
	}
	if !u.IsAbs() {
		return h.appURL + "/" + strings.TrimLeft(destino, "/")
	}
	if base, err := url.Parse(h.appURL); err == nil && u.Scheme == base.Scheme && u.Host == base.Host {
		return destino
	}
	return h.appURL
}
