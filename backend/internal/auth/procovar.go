package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Cliente del hub de identidad de Procovar (procovar-auth).
//
// Existe para que Avisos deje de tener su propia lista de usuarios y contraseñas.
// Quien entra ya está identificado en procovar-auth; aquí solo se comprueba que
// esa sesión es buena y que la persona lleva los permisos de `avisos`.
//
// El login local NO se quita: sigue funcionando en paralelo (ver AdminAuthenticator
// .Middleware). Quitarlo de golpe deja fuera a cualquiera el día que el hub no
// conteste, y un servicio de avisos que no se puede administrar cuando algo va mal
// es justo el que hace falta cuando algo va mal.
type ProcovarAuth struct {
	baseURL    string
	clientID   string
	signingKey []byte
	keyVersion string
	cookieName string
	http       *http.Client
}

// NewProcovarAuth devuelve nil cuando falta configuración: sin él, el
// middleware se comporta exactamente como antes.
func NewProcovarAuth(baseURL, clientID, signingKeyHex, keyVersion, cookieName string) (*ProcovarAuth, error) {
	if baseURL == "" || clientID == "" || signingKeyHex == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(signingKeyHex)
	if err != nil {
		return nil, fmt.Errorf("PROCOVAR_AUTH_SIGNING_KEY no es hexadecimal: %w", err)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("PROCOVAR_AUTH_SIGNING_KEY vacía")
	}
	if keyVersion == "" {
		keyVersion = "1"
	}
	if cookieName == "" {
		cookieName = "procovar.session_token"
	}
	return &ProcovarAuth{
		baseURL:    trimSlash(baseURL),
		clientID:   clientID,
		signingKey: key,
		keyVersion: keyVersion,
		cookieName: cookieName,
		// Corto a propósito: esto se llama en la ruta de cada petición admin.
		// Si el hub tarda, es mejor un 401 rápido que una pantalla colgada.
		http: &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// CookieName es el nombre de la cookie de sesión que emite procovar-auth.
func (p *ProcovarAuth) CookieName() string { return p.cookieName }

// Sesion es lo que devuelve el hub sobre quien está entrando.
type Sesion struct {
	UserID   string
	Email    string
	Nombre   string
	Roles    []string
	Permisos []string
	// TodoVale = `wildcard` del hub: administrador de sistema, lo puede todo.
	TodoVale bool
}

// Puede indica si la sesión lleva una clave concreta del catálogo.
func (s Sesion) Puede(clave string) bool {
	if s.TodoVale {
		return true
	}
	for _, p := range s.Permisos {
		if p == clave {
			return true
		}
	}
	return false
}

type verifySessionResp struct {
	Valid bool `json:"valid"`
	User  struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
	Rbac struct {
		Roles       []string `json:"roles"`
		Permissions []string `json:"permissions"`
		// OJO: el hub lo llama `wildcard`, no `isSystemAdmin`.
		//
		// Para un administrador de sistema, resolveRbac devuelve `global: []` y
		// `wildcard: true` — la lista de permisos viene VACIA a proposito, porque
		// los tiene todos. Leer el nombre equivocado deja la bandera en false, los
		// permisos en cero, y al super admin fuera con un 401 sin explicacion.
		Wildcard bool `json:"wildcard"`
	} `json:"rbac"`
}

// VerifySession valida el valor de la cookie contra procovar-auth y devuelve
// quién es y qué puede.
func (p *ProcovarAuth) VerifySession(ctx context.Context, sessionToken string) (Sesion, error) {
	body, err := json.Marshal(map[string]string{"sessionToken": sessionToken})
	if err != nil {
		return Sesion{}, err
	}
	const path = "/api/auth/verify-session"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return Sesion{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.firmar(req, http.MethodPost, path, body)

	res, err := p.http.Do(req)
	if err != nil {
		return Sesion{}, fmt.Errorf("procovar-auth no responde: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		// Se descarta el cuerpo para no dejar la conexión a medias.
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))
		return Sesion{}, fmt.Errorf("procovar-auth devolvió %d", res.StatusCode)
	}
	var out verifySessionResp
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil {
		return Sesion{}, fmt.Errorf("respuesta ilegible de procovar-auth: %w", err)
	}
	if !out.Valid || out.User.ID == "" {
		return Sesion{}, fmt.Errorf("sesión no válida")
	}
	return Sesion{
		UserID:   out.User.ID,
		Email:    out.User.Email,
		Nombre:   out.User.Name,
		Roles:    out.Rbac.Roles,
		Permisos: out.Rbac.Permissions,
		TodoVale: out.Rbac.Wildcard,
	}, nil
}

// firmar añade las cabeceras de autenticación entre servicios.
//
//	stringToSign = METODO \n ruta \n ts \n nonce \n sha256hex(cuerpo)
//	firma        = hex(HMAC_SHA256(clave, stringToSign))
//
// La clave va HEX-DECODIFICADA: procovar-auth hace `Buffer.from(key, 'hex')`.
// Usarla como texto plano da una firma que siempre falla, y el error que se ve
// es un 401 idéntico al de una cookie mala, que es de los peores de diagnosticar.
func (p *ProcovarAuth) firmar(req *http.Request, method, path string, body []byte) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := uuid.NewString()
	sum := sha256.Sum256(body)
	stringToSign := method + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + hex.EncodeToString(sum[:])

	mac := hmac.New(sha256.New, p.signingKey)
	mac.Write([]byte(stringToSign))

	req.Header.Set("X-Client-Id", p.clientID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", hex.EncodeToString(mac.Sum(nil)))
	req.Header.Set("X-Key-Version", p.keyVersion)
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
