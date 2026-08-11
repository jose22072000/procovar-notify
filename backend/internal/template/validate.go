package template

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"dvtech/qbn/internal/apperr"
)

// schemaCache cachea JSON Schemas ya compilados, indexados por el hash del
// schema. El required_variables es inmutable por versión, así que el contenido
// es una clave estable que se autoinvalida al cambiar. El *jsonschema.Schema
// compilado es de solo lectura: Validate es seguro desde varias goroutines.
var schemaCache sync.Map // [32]byte -> *jsonschema.Schema

// compileSchema compila (con caché) el JSON Schema dado.
func compileSchema(schemaJSON []byte) (*jsonschema.Schema, error) {
	key := sha256.Sum256(schemaJSON)
	if v, ok := schemaCache.Load(key); ok {
		return v.(*jsonschema.Schema), nil
	}

	var schemaDoc any
	if err := json.Unmarshal(schemaJSON, &schemaDoc); err != nil {
		return nil, fmt.Errorf("required_variables no es JSON válido: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	const resourceURL = "schema://template"
	if err := compiler.AddResource(resourceURL, schemaDoc); err != nil {
		return nil, fmt.Errorf("registrando schema: %w", err)
	}
	sch, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("compilando schema: %w", err)
	}
	actual, _ := schemaCache.LoadOrStore(key, sch)
	return actual.(*jsonschema.Schema), nil
}

// ValidatePayload valida el payload contra el JSON Schema dado (el
// required_variables del template). Si schemaJSON está vacío/nil, no valida.
// Devuelve un apperr.Validation (422) si el payload no cumple.
func ValidatePayload(schemaJSON []byte, payload map[string]any) error {
	if len(bytes.TrimSpace(schemaJSON)) == 0 {
		return nil
	}

	sch, err := compileSchema(schemaJSON)
	if err != nil {
		// El schema es dato nuestro (derivado en la Fase 5); si está corrupto o
		// no compila es un error interno, no del cliente.
		return apperr.Internal(err)
	}

	if err := sch.Validate(any(payload)); err != nil {
		return apperr.Validation("payload_validation_failed", summarize(err))
	}
	return nil
}

// summarize reduce el error de validación a un mensaje legible para el cliente.
func summarize(err error) string {
	var ve *jsonschema.ValidationError
	if errors.As(err, &ve) {
		return "Payload does not satisfy the template's required variables"
	}
	return "Payload validation failed"
}
