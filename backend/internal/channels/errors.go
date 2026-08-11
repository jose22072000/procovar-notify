package channels

import "errors"

// PermanentError marca un fallo no recuperable de validación/datos (destinatario
// o payload inválido): no es un fallo de infraestructura, así que NO debe contar
// para abrir el circuit breaker (un destinatario mal formado no significa que la
// conexión SMTP/el proveedor estén caídos).
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent envuelve un error como permanente (de validación/datos).
func Permanent(err error) error { return &PermanentError{Err: err} }

// IsPermanent indica si el error (o alguno que envuelve) es permanente.
func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}

// UnavailableError marca que el destino (SMTP/proveedor) está caído — el
// circuit breaker abierto. No es un fallo del mensaje: el envío debe
// reintentarse con retardo hasta que el destino vuelva, sin consumir
// reintentos ni acabar en FAILED (los mensajes no se pierden por una caída).
type UnavailableError struct{ Err error }

func (e *UnavailableError) Error() string { return e.Err.Error() }
func (e *UnavailableError) Unwrap() error { return e.Err }

// Unavailable envuelve un error como indisponibilidad del destino.
func Unavailable(err error) error { return &UnavailableError{Err: err} }

// IsUnavailable indica si el error (o alguno que envuelve) es de destino caído.
func IsUnavailable(err error) bool {
	var ue *UnavailableError
	return errors.As(err, &ue)
}
