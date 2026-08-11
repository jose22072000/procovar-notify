package notification

import (
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"syscall"

	"dvtech/qbn/internal/safedial"
)

// allowPrivateHosts permite (cuando es true) descargar adjuntos de IPs privadas
// o loopback. Por defecto false: el guard anti-SSRF rechaza esos destinos. Se
// puede habilitar en entornos cerrados/confiables vía SetAllowPrivateHosts.
var allowPrivateHosts atomic.Bool

// SetAllowPrivateHosts habilita/inhabilita el acceso a IPs privadas/loopback en
// la descarga de adjuntos por URL (por defecto deshabilitado). Lo invoca el
// arranque del worker según configuración.
func SetAllowPrivateHosts(v bool) { allowPrivateHosts.Store(v) }

// hostAllowlist, si está configurada (no vacía), restringe las URLs de adjuntos
// a ese conjunto de hosts (defensa en profundidad sobre el guard por IP). Si está
// vacía/sin configurar, no aplica restricción por host. Guarda un
// map[string]struct{} (hosts en minúscula).
var hostAllowlist atomic.Value

// SetHostAllowlist fija la allowlist de hosts para adjuntos por URL. Una lista
// vacía desactiva la restricción. Lo invoca el arranque del worker según
// configuración (ATTACHMENT_HOST_ALLOWLIST).
func SetHostAllowlist(hosts []string) {
	set := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			set[h] = struct{}{}
		}
	}
	hostAllowlist.Store(set)
}

// checkHostAllowed devuelve error si hay allowlist configurada y el host no está
// en ella. Sin allowlist (o vacía) permite cualquier host (sujeto al guard por IP).
func checkHostAllowed(host string) error {
	v := hostAllowlist.Load()
	if v == nil {
		return nil
	}
	set := v.(map[string]struct{})
	if len(set) == 0 {
		return nil
	}
	if _, ok := set[strings.ToLower(host)]; !ok {
		return fmt.Errorf("host no permitido por la allowlist: %s", host)
	}
	return nil
}

// ssrfControl es el Control del net.Dialer: se ejecuta con la dirección ya
// resuelta justo antes de conectar, de modo que bloquea por IP real (evita el
// bypass por DNS que apunta a una IP interna).
func ssrfControl(_, address string, _ syscall.RawConn) error {
	if allowPrivateHosts.Load() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("dirección no resoluble: %s", host)
	}
	if isBlockedIP(ip) {
		return fmt.Errorf("destino no permitido (IP privada/loopback/link-local): %s", host)
	}
	return nil
}

// isBlockedIP delega en el guard compartido (loopback, no especificada,
// link-local, multicast y rangos privados IPv4/IPv6).
func isBlockedIP(ip net.IP) bool {
	return safedial.IsBlockedIP(ip)
}
