package notification

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
)

// MaxBatchRecipients es el tope de destinatarios por llamada de envío masivo.
const MaxBatchRecipients = 1000

// BatchItem es un destinatario del envío masivo (con su recipient/payload).
type BatchItem struct {
	Recipient       map[string]any
	RecipientUserID *string
	Payload         map[string]any
	IdempotencyKey  *string
}

// BatchInput describe un envío masivo: template/tipo/canal comunes + N items.
type BatchInput struct {
	ApplicationID    uuid.UUID
	TemplateKey      string
	NotificationType string
	Channel          string
	Priority         string
	Locale           *string
	Items            []BatchItem
}

// BatchFailure indica el fallo de un item concreto (por índice).
type BatchFailure struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// BatchResult es el resultado con éxito parcial.
type BatchResult struct {
	Created []string       `json:"created"` // ids de notificaciones creadas
	Failed  []BatchFailure `json:"failed"`
}

// CreateBatch crea una notificación por item reutilizando Create. Devuelve éxito
// parcial: cada item falla de forma aislada sin abortar el resto.
func (s *Service) CreateBatch(ctx context.Context, in BatchInput) (BatchResult, error) {
	if len(in.Items) == 0 {
		return BatchResult{}, apperr.Validation("empty_batch", "batch requires at least one recipient")
	}
	if len(in.Items) > MaxBatchRecipients {
		return BatchResult{}, apperr.Validation("batch_too_large",
			"batch exceeds the maximum of "+strconv.Itoa(MaxBatchRecipients)+" recipients")
	}
	// Los masivos no usan la cola critical: URGENT/HIGH se rechaza explícito
	// (para urgencias reales, envíos individuales).
	if in.Priority == "URGENT" || in.Priority == "HIGH" {
		return BatchResult{}, apperr.Validation("batch_priority_not_allowed",
			"batch does not allow URGENT/HIGH priority; use NORMAL/LOW, or individual sends for truly urgent notifications")
	}

	res := BatchResult{Created: []string{}, Failed: []BatchFailure{}}
	for i, item := range in.Items {
		n, err := s.Create(ctx, CreateInput{
			ApplicationID:    in.ApplicationID,
			TemplateKey:      in.TemplateKey,
			NotificationType: in.NotificationType,
			Channel:          in.Channel,
			Recipient:        item.Recipient,
			RecipientUserID:  item.RecipientUserID,
			Payload:          item.Payload,
			Priority:         in.Priority,
			Locale:           in.Locale,
			IdempotencyKey:   item.IdempotencyKey,
		})
		if err != nil {
			res.Failed = append(res.Failed, BatchFailure{Index: i, Error: apperr.From(err).Detail})
			continue
		}
		res.Created = append(res.Created, n.ID.String())
	}
	return res, nil
}
