package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
)

// maxSignedBody limita el cuerpo que se lee para calcular su hash (10 MiB,
// alineado con el límite del body parser).
const maxSignedBody = 10 << 20

// APIKeyQuerier es la porción del store que necesita la autenticación HMAC.
// Definirla aquí (en el consumidor) evita acoplar auth a toda la capa de datos.
type APIKeyQuerier interface {
	GetAPIKeyByKeyID(ctx context.Context, keyID string) (sqlc.ApiKey, error)
	// TouchAPIKeyLastUsed sella el último uso de la credencial. La columna existía
	// y el panel la pintaba, pero nadie la escribía: «Último uso» salía siempre «—».
	TouchAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error
}

// HMACAuthenticator autentica peticiones de la API pública por firma HMAC (§4.1).
type HMACAuthenticator struct {
	q      APIKeyQuerier
	enc    *crypto.Encryptor
	skew   time.Duration
	now    func() time.Time // inyectable para tests
	replay ReplayGuard      // opcional: caché de nonce anti-replay (nil = sin caché)
}

// NewHMACAuthenticator crea el autenticador con la ventana anti-replay indicada.
func NewHMACAuthenticator(q APIKeyQuerier, enc *crypto.Encryptor, skewSeconds int) *HMACAuthenticator {
	return &HMACAuthenticator{
		q:    q,
		enc:  enc,
		skew: time.Duration(skewSeconds) * time.Second,
		now:  time.Now,
	}
}

// WithReplayGuard activa la caché de nonce: además de la ventana de timestamp,
// rechaza reenvíos exactos de una firma ya usada. Devuelve el mismo autenticador
// para encadenar en la construcción.
func (a *HMACAuthenticator) WithReplayGuard(g ReplayGuard) *HMACAuthenticator {
	a.replay = g
	return a
}

// Middleware verifica la firma y, si es válida, inyecta el Caller (con el
// application_id del tenant) en el contexto.
func (a *HMACAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := a.authenticate(r)
		if err != nil {
			httpx.WriteProblem(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(ContextWithCaller(r.Context(), caller)))
	})
}

// unauthorized es el error genérico de firma inválida. Se usa el mismo mensaje
// para todos los fallos de firma/credencial para no dar pistas a un atacante.
var unauthorized = apperr.Unauthorized("invalid_signature", "Invalid or missing request signature")

// authenticate valida las cabeceras HMAC y devuelve la identidad del tenant.
// Restaura r.Body para que los handlers puedan volver a leer el cuerpo.
func (a *HMACAuthenticator) authenticate(r *http.Request) (Caller, error) {
	keyID := r.Header.Get(HeaderKeyID)
	tsStr := r.Header.Get(HeaderTimestamp)
	signature := r.Header.Get(HeaderSignature)
	if keyID == "" || tsStr == "" || signature == "" {
		return Caller{}, unauthorized
	}

	// Ventana anti-replay: el timestamp no puede desviarse más de `skew`.
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return Caller{}, unauthorized
	}
	if drift := a.now().Unix() - ts; drift > int64(a.skew.Seconds()) || drift < -int64(a.skew.Seconds()) {
		return Caller{}, apperr.Unauthorized("signature_expired", "Request timestamp outside the allowed window")
	}

	// Lee el cuerpo para hashearlo y lo restaura para el handler.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxSignedBody))
	if err != nil {
		return Caller{}, apperr.Internal(err)
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Localiza la credencial. Si no existe, se responde como firma inválida.
	key, err := a.q.GetAPIKeyByKeyID(r.Context(), keyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Caller{}, unauthorized
		}
		return Caller{}, apperr.Internal(err)
	}

	if key.Status != sqlc.StatusActiveRevokedACTIVE {
		return Caller{}, apperr.Forbidden("key_revoked", "API key is revoked")
	}
	if key.ExpiresAt.Valid && key.ExpiresAt.Time.Before(a.now()) {
		return Caller{}, apperr.Forbidden("key_expired", "API key has expired")
	}

	secret, err := a.enc.Decrypt(key.SecretEnc)
	if err != nil {
		return Caller{}, apperr.Internal(err)
	}

	if !verifySignature(secret, r.Method, r.URL.Path, CanonicalQuery(r.URL), body, tsStr, signature) {
		return Caller{}, unauthorized
	}

	// Anti-replay: solo tras validar la firma (para que un atacante no pueda
	// llenar la caché con basura). La firma es única por petición y sirve de
	// nonce. TTL = 2×skew para cubrir toda la ventana en la que ese mismo
	// timestamp seguiría siendo aceptable.
	if a.replay != nil {
		first, err := a.replay.FirstSeen(r.Context(), signature, 2*a.skew)
		switch {
		case err != nil:
			// Falla abierta, como el rate limiter: una caída de Redis no debe
			// tumbar la API pública; la ventana de timestamp sigue acotando el
			// margen de replay.
		case !first:
			return Caller{}, apperr.Unauthorized("signature_replayed", "Request signature has already been used")
		}
	}

	// Sella el último uso de la key (columna «Último uso» del panel). Asíncrono y
	// con throttle de un minuto: escribir en cada petición sería amplificación de
	// escrituras inútil, y un fallo aquí no debe tumbar una petición ya válida.
	if !key.LastUsedAt.Valid || a.now().Sub(key.LastUsedAt.Time) > time.Minute {
		go func(id uuid.UUID) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = a.q.TouchAPIKeyLastUsed(ctx, id)
		}(key.ID)
	}

	return Caller{
		ApplicationID: key.ApplicationID,
		APIKeyID:      key.ID,
		KeyID:         key.KeyID,
		Scopes:        key.Scopes,
	}, nil
}

// RequireScope es un middleware que exige que la API key autenticada posea el
// scope dado. Debe montarse después de HMACAuthenticator.Middleware.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			caller, ok := CallerFromContext(r.Context())
			if !ok {
				httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
				return
			}
			if !caller.HasScope(scope) {
				httpx.WriteProblem(w, r, apperr.Forbidden("missing_scope", "Missing required scope: "+scope))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
