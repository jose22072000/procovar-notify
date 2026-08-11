package template

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// Modos de edición de una plantilla.
const (
	KindBuilder = "builder" // editor visual por bloques (structure → body)
	KindHTML    = "html"    // HTML/CSS crudo saneado (body directo)
)

// Service gestiona los templates por aplicación (versionado inmutable) y la
// librería global de BaseTemplate. Compila la estructura del builder a HTML y
// deriva las variables requeridas.
type Service struct {
	db     *store.DB
	logger *slog.Logger
}

// NewService crea el servicio de templates.
func NewService(db *store.DB, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// Input es la entrada para crear o actualizar un template.
type Input struct {
	Key            string
	Channel        string
	Locale         string
	Name           string
	Description    string
	Subject        string
	Structure      json.RawMessage // secciones del builder; vacío => clonar de base
	BaseTemplateID *uuid.UUID
	Variables      map[string]VariableSpec // overrides de variables derivadas
	Kind           string                  // "builder" (por defecto) o "html"
	Body           string                  // HTML crudo cuando Kind == "html"
}

// Create crea un template NUEVO (versión 1). Falla si ya existe (app,key,locale).
func (s *Service) Create(ctx context.Context, actor, appID uuid.UUID, in Input) (sqlc.Template, error) {
	in.applyDefaults()

	max, err := s.db.Q.GetMaxTemplateVersion(ctx, sqlc.GetMaxTemplateVersionParams{
		ApplicationID: appID, Key: in.Key, Locale: in.Locale,
	})
	if err != nil {
		return sqlc.Template{}, apperr.Internal(err)
	}
	if max > 0 {
		return sqlc.Template{}, apperr.Conflict("template_exists",
			"A template with this key/locale already exists; edit it to create a new version")
	}

	if in.Kind == KindHTML {
		structure, baseID := parseStructureOpt(in.Structure), baseIDFrom(in.BaseTemplateID)
		return s.persistVersion(ctx, actor, appID, in, structure, baseID, audit.ActionTemplateCreate)
	}

	structure, baseID, err := s.resolveStructure(ctx, in)
	if err != nil {
		return sqlc.Template{}, err
	}
	return s.persistVersion(ctx, actor, appID, in, structure, baseID, audit.ActionTemplateCreate)
}

// Update crea una NUEVA versión activa de un template existente (las anteriores
// quedan inactivas pero consultables). Identifica el template por id.
func (s *Service) Update(ctx context.Context, actor, appID, id uuid.UUID, in Input) (sqlc.Template, error) {
	current, err := s.Get(ctx, appID, id)
	if err != nil {
		return sqlc.Template{}, err
	}

	// La clave/locale/canal no cambian al versionar; se heredan del actual.
	in.Key = current.Key
	in.Locale = current.Locale
	in.Channel = string(current.Channel)
	if in.Name == "" {
		in.Name = current.Name
	}
	if in.Subject == "" {
		in.Subject = derefStr(current.Subject)
	}
	if len(in.Structure) == 0 {
		in.Structure = current.Structure // reusar la estructura actual si no se cambia
	}
	if current.BaseTemplateID.Valid {
		bid := uuid.UUID(current.BaseTemplateID.Bytes)
		in.BaseTemplateID = &bid
	}
	if in.Kind == "" {
		in.Kind = current.Kind // el modo no cambia al versionar salvo conversión explícita
	}

	if in.Kind == KindHTML {
		if strings.TrimSpace(in.Body) == "" {
			in.Body = current.Body // si no se reenvía el HTML, se reusa el actual
		}
		// structure conserva los bloques (para "volver a visual"); baseID heredado.
		return s.persistVersion(ctx, actor, appID, in, parseStructureOpt(in.Structure), baseIDFrom(in.BaseTemplateID), audit.ActionTemplateUpdate)
	}

	structure, baseID, err := s.resolveStructure(ctx, in)
	if err != nil {
		return sqlc.Template{}, err
	}
	return s.persistVersion(ctx, actor, appID, in, structure, baseID, audit.ActionTemplateUpdate)
}

// persistVersion compila, deriva variables y persiste una versión activa,
// desactivando las anteriores de la misma (key, locale) en la misma transacción.
// La versión (max+1) se calcula DENTRO de la transacción para evitar un TOCTOU
// entre el cálculo y el INSERT con dos escrituras concurrentes.
func (s *Service) persistVersion(ctx context.Context, actor, appID uuid.UUID, in Input, structure Structure, baseID pgtype.UUID, action audit.Action) (sqlc.Template, error) {
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	if in.Kind != KindHTML {
		in.Kind = KindBuilder // garantía: nunca se persiste un kind fuera de la CHECK
	}
	body, vars, err := s.compileBody(in, structure)
	if err != nil {
		return sqlc.Template{}, err
	}
	required, err := BuildRequiredSchema(vars, in.Variables)
	if err != nil {
		return sqlc.Template{}, apperr.Internal(err)
	}
	structureJSON, _ := json.Marshal(structure)

	var created sqlc.Template
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		max, err := q.GetMaxTemplateVersion(ctx, sqlc.GetMaxTemplateVersionParams{
			ApplicationID: appID, Key: in.Key, Locale: in.Locale,
		})
		if err != nil {
			return err
		}
		if err := q.DeactivateTemplateVersions(ctx, sqlc.DeactivateTemplateVersionsParams{
			ApplicationID: appID, Key: in.Key, Locale: in.Locale,
		}); err != nil {
			return err
		}
		t, err := q.CreateTemplateFull(ctx, sqlc.CreateTemplateFullParams{
			ApplicationID:     appID,
			Key:               in.Key,
			Channel:           sqlc.Channel(in.Channel),
			BaseTemplateID:    baseID,
			Name:              in.Name,
			Description:       nilIfEmpty(in.Description),
			Subject:           nilIfEmpty(in.Subject),
			Structure:         structureJSON,
			Body:              body,
			RequiredVariables: required,
			Locale:            in.Locale,
			Version:           max + 1,
			Kind:              in.Kind,
		})
		if err != nil {
			return err
		}
		created = t
		appIDCopy := appID
		details, _ := json.Marshal(map[string]any{"key": t.Key, "locale": t.Locale, "version": t.Version})
		return audit.Record(ctx, q, audit.Entry{
			ActorAdminID:  actor,
			ApplicationID: &appIDCopy,
			Action:        action,
			TargetType:    "template",
			TargetID:      t.ID.String(),
			Details:       details,
		})
	})
	if err != nil {
		return sqlc.Template{}, store.MapError(err, "", "template_conflict")
	}
	return created, nil
}

// compileBody produce el `body` HTML y las variables según el modo:
//   - html: sanea el HTML crudo y extrae variables de subject+body.
//   - builder: compila la estructura de bloques y deriva variables de ella.
func (s *Service) compileBody(in Input, structure Structure) (string, []string, error) {
	if in.Kind == KindHTML {
		if in.Channel != "EMAIL" {
			return "", nil, apperr.Validation("html_email_only", "advanced HTML mode is only available for EMAIL")
		}
		if strings.TrimSpace(in.Body) == "" {
			return "", nil, apperr.Validation("missing_body", "body is required for html templates")
		}
		sanitized, err := SanitizeEmailHTML(in.Body)
		if err != nil {
			return "", nil, apperr.Validation("invalid_html", err.Error())
		}
		return neutralizeTripleStache(sanitized), DeriveVariablesFromText(in.Subject, in.Body), nil
	}
	body, err := CompileForChannel(in.Channel, structure)
	if err != nil {
		return "", nil, apperr.Validation("invalid_structure", err.Error())
	}
	return body, DeriveVariables(structure, in.Subject), nil
}

// parseStructureOpt parsea una estructura opcional (bloques conservados para el
// modo html); si está vacía o es inválida, devuelve una estructura vacía.
func parseStructureOpt(raw json.RawMessage) Structure {
	if len(raw) == 0 {
		return Structure{}
	}
	s, err := ParseStructure(raw)
	if err != nil {
		return Structure{}
	}
	return s
}

// baseIDFrom convierte un *uuid.UUID en pgtype.UUID (inválido si es nil).
func baseIDFrom(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// resolveStructure obtiene la estructura: la del input, o la clonada del
// BaseTemplate si el input no trae estructura.
func (s *Service) resolveStructure(ctx context.Context, in Input) (Structure, pgtype.UUID, error) {
	var baseID pgtype.UUID
	raw := in.Structure

	if len(raw) == 0 && in.BaseTemplateID != nil {
		base, err := s.db.Q.GetBaseTemplate(ctx, *in.BaseTemplateID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Structure{}, baseID, apperr.NotFound("base_template_not_found", "Base template not found")
		}
		if err != nil {
			return Structure{}, baseID, apperr.Internal(err)
		}
		raw = base.Structure
	}
	if in.BaseTemplateID != nil {
		baseID = pgtype.UUID{Bytes: *in.BaseTemplateID, Valid: true}
	}
	if len(raw) == 0 {
		return Structure{}, baseID, apperr.Validation("missing_structure", "structure or baseTemplateId is required")
	}

	structure, err := ParseStructure(raw)
	if err != nil {
		return Structure{}, baseID, apperr.Validation("invalid_structure", err.Error())
	}
	return structure, baseID, nil
}

// Get devuelve un template del tenant (404 si no existe).
func (s *Service) Get(ctx context.Context, appID, id uuid.UUID) (sqlc.Template, error) {
	t, err := s.db.Q.GetTemplateByID(ctx, sqlc.GetTemplateByIDParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Template{}, apperr.NotFound("template_not_found", "Template not found")
	}
	if err != nil {
		return sqlc.Template{}, apperr.Internal(err)
	}
	return t, nil
}

// List devuelve los templates activos del tenant.
func (s *Service) List(ctx context.Context, appID uuid.UUID) ([]sqlc.Template, error) {
	ts, err := s.db.Q.ListActiveTemplates(ctx, appID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return ts, nil
}

// Delete elimina una plantilla completa (todas sus versiones) resuelta por el id
// de una de sus filas. Registra la acción en auditoría.
func (s *Service) Delete(ctx context.Context, actor, appID, id uuid.UUID) error {
	t, err := s.Get(ctx, appID, id)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteTemplatesByKey(ctx, sqlc.DeleteTemplatesByKeyParams{ApplicationID: appID, Key: t.Key, Locale: t.Locale}); err != nil {
			return apperr.Internal(err)
		}
		appIDCopy := appID
		details, _ := json.Marshal(map[string]any{"key": t.Key, "locale": t.Locale})
		return audit.Record(ctx, q, audit.Entry{
			ActorAdminID:  actor,
			ApplicationID: &appIDCopy,
			Action:        audit.ActionTemplateDelete,
			TargetType:    "template",
			TargetID:      id.String(),
			Details:       details,
		})
	})
}

// PreviewResult es el resultado de un preview con payload de prueba.
type PreviewResult struct {
	Subject          string   `json:"subject"`
	HTML             string   `json:"html"` // renderizado con el payload (para el iframe)
	Body             string   `json:"body"` // compilado SIN renderizar (con {{vars}}); sirve para "pasar a HTML"
	MissingVariables []string `json:"missingVariables"`
}

// Preview renderiza el template con un payload de prueba e informa de las
// variables requeridas que falten (sin fallar: es una previsualización).
func (s *Service) Preview(ctx context.Context, appID, id uuid.UUID, payload map[string]any) (PreviewResult, error) {
	t, err := s.Get(ctx, appID, id)
	if err != nil {
		return PreviewResult{}, err
	}
	subject, err := Render(derefStr(t.Subject), payload)
	if err != nil {
		return PreviewResult{}, apperr.Validation("render_error", err.Error())
	}
	html, err := Render(t.Body, payload)
	if err != nil {
		return PreviewResult{}, apperr.Validation("render_error", err.Error())
	}
	return PreviewResult{
		Subject:          subject,
		HTML:             html,
		Body:             t.Body,
		MissingVariables: missingRequired(t.RequiredVariables, payload),
	}, nil
}

// PreviewDraft renderiza una estructura SIN guardar (editor en vivo): compila
// según el canal, renderiza subject+body con el payload de prueba e informa de
// las variables requeridas ausentes, derivadas de la propia estructura.
func (s *Service) PreviewDraft(channel, kind string, structureRaw json.RawMessage, subject, rawBody string, variables map[string]VariableSpec, payload map[string]any) (PreviewResult, error) {
	var body string
	var vars []string
	if kind == KindHTML {
		sanitized, err := SanitizeEmailHTML(rawBody)
		if err != nil {
			return PreviewResult{}, apperr.Validation("invalid_html", err.Error())
		}
		body = neutralizeTripleStache(sanitized)
		vars = DeriveVariablesFromText(subject, rawBody)
	} else {
		structure, err := ParseStructure(structureRaw)
		if err != nil {
			return PreviewResult{}, apperr.Validation("invalid_structure", err.Error())
		}
		compiled, err := CompileForChannelPreview(channel, structure)
		if err != nil {
			return PreviewResult{}, apperr.Validation("invalid_structure", err.Error())
		}
		body = compiled
		vars = DeriveVariables(structure, subject)
	}

	subjectOut, err := Render(subject, payload)
	if err != nil {
		return PreviewResult{}, apperr.Validation("render_error", err.Error())
	}
	bodyOut, err := Render(body, payload)
	if err != nil {
		return PreviewResult{}, apperr.Validation("render_error", err.Error())
	}
	required, _ := BuildRequiredSchema(vars, variables)
	return PreviewResult{
		Subject:          subjectOut,
		HTML:             bodyOut,
		Body:             body,
		MissingVariables: missingRequired(required, payload),
	}, nil
}

// ListBaseTemplates devuelve la librería global (solo lectura en esta fase).
func (s *Service) ListBaseTemplates(ctx context.Context) ([]sqlc.BaseTemplate, error) {
	bs, err := s.db.Q.ListBaseTemplates(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return bs, nil
}

func (in *Input) applyDefaults() {
	if in.Channel == "" {
		in.Channel = "EMAIL"
	}
	if in.Locale == "" {
		in.Locale = "en"
	}
	if in.Kind != KindHTML {
		in.Kind = KindBuilder // cualquier valor no reconocido cae en builder
	}
}

// missingRequired devuelve las variables requeridas por el schema que no están
// presentes en el payload.
func missingRequired(schemaJSON []byte, payload map[string]any) []string {
	if len(schemaJSON) == 0 {
		return nil
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil
	}
	var missing []string
	for _, r := range schema.Required {
		if _, ok := payload[r]; !ok {
			missing = append(missing, r)
		}
	}
	return missing
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
