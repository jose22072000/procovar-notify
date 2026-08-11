package channels

import (
	"context"

	"dvtech/qbn/internal/domain"
)

// InAppSender "entrega" notificaciones in-app. No hay proveedor externo: la
// propia fila de la notificación es la entrada de bandeja, consultable por la
// API (GET /v1/inbox, Fase 4). Aquí solo confirma la entrega.
type InAppSender struct{}

// NewInAppSender crea el sender in-app.
func NewInAppSender() *InAppSender { return &InAppSender{} }

func (s *InAppSender) Channel() string { return domain.ChannelInApp }

func (s *InAppSender) Send(_ context.Context, _ RenderedMessage, _ ResolvedRoute) (ProviderRef, error) {
	return ProviderRef("inapp"), nil
}
