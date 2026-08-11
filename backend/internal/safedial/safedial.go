// Package safedial proporciona un guard anti-SSRF reutilizable: un net.Dialer
// (y un http.Client) que rechaza, por la IP ya resuelta justo antes de conectar,
// destinos loopback/privados/link-local. Validar por IP resuelta evita el bypass
// por DNS (un host público que resuelve a una IP interna) y cubre también los
// redirects, porque cada salto vuelve a pasar por el dialer.
package safedial

import (
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"syscall"
	"time"
)

// allowPrivate, cuando es true, desactiva el guard (permite destinos internos).
// Por defecto false. Se habilita en entornos cerrados/confiables (p. ej. webhooks
// a un backend en la misma red privada) vía SetAllowPrivate.
var allowPrivate atomic.Bool

// SetAllowPrivate habilita/inhabilita el acceso a IPs privadas/loopback en los
// clientes construidos con NewClient (por defecto deshabilitado).
func SetAllowPrivate(v bool) { allowPrivate.Store(v) }

// IsBlockedIP indica si una IP no debe alcanzarse desde una petición saliente
// no confiable: loopback, no especificada, link-local, multicast y rangos
// privados (IPv4 e IPv6 unique-local fc00::/7).
func IsBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate()
}

// Control es el Control del net.Dialer: se ejecuta con la dirección ya resuelta
// justo antes de conectar, de modo que bloquea por la IP real.
func Control(_, address string, _ syscall.RawConn) error {
	if allowPrivate.Load() {
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
	if IsBlockedIP(ip) {
		return fmt.Errorf("destino no permitido (IP privada/loopback/link-local): %s", host)
	}
	return nil
}

// NewClient construye un http.Client con el guard anti-SSRF aplicado en el dialer.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
				Control: Control,
			}).DialContext,
		},
	}
}
