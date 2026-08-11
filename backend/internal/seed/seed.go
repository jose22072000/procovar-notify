// Package seed siembra datos iniciales idempotentes: la librería global de
// plantillas base (13 de bloques + 12 HTML), un super-admin y, opcionalmente, una
// aplicación demo. Es importable para que lo usen tanto cmd/seed como el arranque
// del api (siembra automática si la BD está limpia).
package seed

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/template"
)

// htmlBasesFS embebe las 12 plantillas HTML por defecto (modo avanzado) + su índice.
//
//go:embed htmlbases/*.html htmlbases/index.json
var htmlBasesFS embed.FS

// logoLightPNG es el logo de marca en negro (modo light) para las plantillas base.
//
//go:embed assets/logo-light.png
var logoLightPNG []byte

// logoToken es el marcador que en las plantillas base se sustituye por el logo.
const logoToken = "__BRAND_LOGO__"

// Options configura la siembra.
type Options struct {
	AdminEmail    string
	AdminPassword string
	// IncludeDemo crea la aplicación demo (típicamente solo en desarrollo).
	IncludeDemo bool
	// Encryptor + APIKeySecret + SMTPHost: si se proveen (y IncludeDemo), se crea
	// la mensajería del demo (SMTP MailHog + ruta + API key con secreto conocido).
	Encryptor    *crypto.Encryptor
	APIKeySecret string
	SMTPHost     string
}

// Run siembra las plantillas base + el super-admin (idempotente) y, si IncludeDemo,
// la aplicación demo. Todo con ON CONFLICT, así que es seguro re-ejecutarlo.
func Run(ctx context.Context, pool *pgxpool.Pool, opts Options) error {
	if err := seedBaseTemplates(ctx, pool); err != nil {
		return fmt.Errorf("base templates: %w", err)
	}
	if err := seedHTMLBaseTemplates(ctx, pool); err != nil {
		return fmt.Errorf("html base templates: %w", err)
	}
	if err := seedSuperAdmin(ctx, pool, opts.AdminEmail, opts.AdminPassword); err != nil {
		return fmt.Errorf("super-admin: %w", err)
	}
	if opts.IncludeDemo {
		appID, err := seedDemoApp(ctx, pool)
		if err != nil {
			return fmt.Errorf("app demo: %w", err)
		}
		if opts.Encryptor != nil && opts.APIKeySecret != "" {
			host := opts.SMTPHost
			if host == "" {
				host = "localhost"
			}
			if err := seedDemoMessaging(ctx, pool, opts.Encryptor, appID, opts.APIKeySecret, host); err != nil {
				return fmt.Errorf("mensajería demo: %w", err)
			}
		}
	}
	return nil
}

// brandLogoDataURI devuelve el logo embebido como data URI (image/png).
func brandLogoDataURI() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(logoLightPNG)
}

// seedDemoMessaging crea una conexión SMTP a MailHog, una ruta por defecto
// transactional/EMAIL y una API key con secreto conocido (todo idempotente).
func seedDemoMessaging(ctx context.Context, pool *pgxpool.Pool, enc *crypto.Encryptor, appID, secret, smtpHost string) error {
	emptyPwd, err := enc.Encrypt([]byte(""))
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO smtp_connections (application_id, name, host, port, username, password_enc, from_email, from_name, secure)
		VALUES ($1,'mailhog',$2,1025,'',$3,'noreply@demo.local','Demo',false)
		ON CONFLICT (application_id, name) DO NOTHING`, appID, smtpHost, emptyPwd); err != nil {
		return err
	}

	var smtpID string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM smtp_connections WHERE application_id=$1 AND name='mailhog'`, appID).Scan(&smtpID); err != nil {
		return err
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO channel_routes (application_id, notification_type, channel, template_key, smtp_connection_id)
		VALUES ($1,'transactional','EMAIL','welcome',$2)
		ON CONFLICT (application_id, notification_type) DO NOTHING`, appID, smtpID); err != nil {
		return err
	}

	secretEnc, err := enc.Encrypt([]byte(secret))
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO api_keys (application_id, key_id, secret_enc, scopes)
		VALUES ($1,'demo-key',$2,ARRAY['notifications:send','notifications:read'])
		ON CONFLICT (key_id) DO NOTHING`, appID, secretEnc)
	return err
}

// seedBaseTemplates inserta la librería global de plantillas predefinidas (bloques).
func seedBaseTemplates(ctx context.Context, pool *pgxpool.Pool) error {
	type bt struct {
		key, category, name, description, structure, suggested string
	}
	// tema común de la librería (color primario de la marca, emerald).
	const theme = `{"theme":{"primaryColor":"#10b981"},"sections":[`
	items := []bt{
		{
			key: "verify-email", category: "transactional", name: "Verificación de correo",
			description: "Confirma la dirección de correo tras el registro.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, gracias por registrarte en {{appName}}. Confirma tu correo para activar tu cuenta."}},` +
				`{"id":"s3","type":"text","props":{"text":"Tu código de verificación: {{verificationCode}}"}},` +
				`{"id":"s4","type":"button","props":{"text":"Verificar correo","url":"{{verificationUrl}}"}},` +
				`{"id":"s5","type":"text","props":{"text":"Este enlace caduca en {{expirationTime}}. Si no creaste una cuenta, puedes ignorar este correo."}},` +
				`{"id":"s6","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","verificationCode","verificationUrl","expirationTime","year"]`,
		},
		{
			key: "welcome", category: "transactional", name: "Bienvenida",
			description: "Email de bienvenida tras el registro.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, tu cuenta se ha creado correctamente. ¡Nos alegra tenerte a bordo!"}},` +
				`{"id":"s3","type":"button","props":{"text":"Empezar","url":"{{actionUrl}}"}},` +
				`{"id":"s4","type":"text","props":{"text":"Si tienes cualquier duda, escríbenos: estamos para ayudarte."}},` +
				`{"id":"s5","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","actionUrl","year"]`,
		},
		{
			key: "password-reset", category: "transactional", name: "Recuperar contraseña",
			description: "Enlace y código para restablecer la contraseña.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, hemos recibido una solicitud para restablecer la contraseña de tu cuenta de {{appName}}."}},` +
				`{"id":"s3","type":"text","props":{"text":"Tu código de recuperación: {{resetCode}}"}},` +
				`{"id":"s4","type":"button","props":{"text":"Restablecer contraseña","url":"{{resetUrl}}"}},` +
				`{"id":"s5","type":"text","props":{"text":"Este enlace caduca en {{expirationTime}}. Si no lo solicitaste, ignora este correo: tu cuenta sigue segura."}},` +
				`{"id":"s6","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","resetCode","resetUrl","expirationTime","year"]`,
		},
		{
			key: "password-changed", category: "security", name: "Contraseña actualizada",
			description: "Confirma que la contraseña se ha cambiado.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, la contraseña de tu cuenta de {{appName}} se ha actualizado correctamente."}},` +
				`{"id":"s3","type":"text","props":{"text":"Cambio realizado el {{changedAt}} desde la IP {{ipAddress}}."}},` +
				`{"id":"s4","type":"button","props":{"text":"Iniciar sesión","url":"{{loginUrl}}"}},` +
				`{"id":"s5","type":"text","props":{"text":"¿No fuiste tú? Tu cuenta podría estar en riesgo. Contacta con soporte de inmediato en {{supportEmail}}."}},` +
				`{"id":"s6","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","changedAt","ipAddress","loginUrl","supportEmail","year"]`,
		},
		{
			key: "otp", category: "security", name: "Código de verificación",
			description: "Código de un solo uso (OTP) para iniciar sesión.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"Código de verificación"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, usa este código para completar tu acceso a {{appName}}:"}},` +
				`{"id":"s3","type":"header","props":{"title":"{{otpCode}}"}},` +
				`{"id":"s4","type":"text","props":{"text":"Este código caduca en {{expirationTime}}. Nunca lo compartas con nadie; nuestro equipo jamás te lo pedirá."}},` +
				`{"id":"s5","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","otpCode","expirationTime","year"]`,
		},
		{
			key: "invitation", category: "transactional", name: "Invitación",
			description: "Invitación para unirse a una organización.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola, {{inviterName}} te ha invitado a unirte a {{organizationName}} en {{appName}} como {{role}}."}},` +
				`{"id":"s3","type":"button","props":{"text":"Aceptar invitación","url":"{{invitationUrl}}"}},` +
				`{"id":"s4","type":"text","props":{"text":"Esta invitación caduca en {{expirationTime}}. Si no la esperabas, puedes ignorar este correo."}},` +
				`{"id":"s5","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","inviterName","organizationName","role","invitationUrl","expirationTime","year"]`,
		},
		{
			key: "magic-link", category: "transactional", name: "Enlace de acceso",
			description: "Inicio de sesión sin contraseña mediante enlace mágico.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, pulsa el botón para iniciar sesión en {{appName}}:"}},` +
				`{"id":"s3","type":"button","props":{"text":"Iniciar sesión","url":"{{magicLinkUrl}}"}},` +
				`{"id":"s4","type":"text","props":{"text":"Este enlace caduca en {{expirationTime}} y solo puede usarse una vez. Si no lo solicitaste, ignora este correo."}},` +
				`{"id":"s5","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","magicLinkUrl","expirationTime","year"]`,
		},
		{
			key: "account-deletion", category: "security", name: "Eliminar cuenta",
			description: "Confirmación de la eliminación de la cuenta (acción irreversible).",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, hemos recibido una solicitud para eliminar tu cuenta de {{appName}}. Esta acción es irreversible y borrará tu perfil, tus datos y tu historial."}},` +
				`{"id":"s3","type":"button","props":{"text":"Confirmar eliminación","url":"{{confirmationUrl}}"}},` +
				`{"id":"s4","type":"text","props":{"text":"Este enlace caduca en {{expirationTime}}. Si no lo solicitaste, ignora este correo y tu cuenta seguirá activa."}},` +
				`{"id":"s5","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","confirmationUrl","expirationTime","year"]`,
		},
		{
			key: "email-change", category: "security", name: "Cambio de correo",
			description: "Confirmación de la nueva dirección de correo.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, se ha solicitado cambiar el correo de tu cuenta de {{appName}}."}},` +
				`{"id":"s3","type":"text","props":{"text":"Correo actual: {{currentEmail}}\nCorreo nuevo: {{newEmail}}"}},` +
				`{"id":"s4","type":"button","props":{"text":"Confirmar cambio","url":"{{confirmationUrl}}"}},` +
				`{"id":"s5","type":"text","props":{"text":"Este enlace caduca en {{expirationTime}}. ¿No fuiste tú? Cambia tu contraseña de inmediato."}},` +
				`{"id":"s6","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","currentEmail","newEmail","confirmationUrl","expirationTime","year"]`,
		},
		{
			key: "security-alert", category: "security", name: "Alerta de seguridad",
			description: "Aviso de actividad sospechosa en la cuenta.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"Alerta de seguridad"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, hemos detectado actividad inusual en tu cuenta de {{appName}}: {{alertType}}."}},` +
				`{"id":"s3","type":"text","props":{"text":"Fecha: {{eventTime}}\nUbicación: {{location}}\nDispositivo: {{device}}\nIP: {{ipAddress}}"}},` +
				`{"id":"s4","type":"button","props":{"text":"Proteger mi cuenta","url":"{{secureAccountUrl}}"}},` +
				`{"id":"s5","type":"text","props":{"text":"Si fuiste tú, no hace falta que hagas nada. Si no, cambia tu contraseña y activa la verificación en dos pasos."}},` +
				`{"id":"s6","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","alertType","eventTime","location","device","ipAddress","secureAccountUrl","year"]`,
		},
		{
			key: "2fa-enabled", category: "security", name: "2FA activado",
			description: "Confirmación de que se activó la verificación en dos pasos.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, has activado la verificación en dos pasos (2FA) en tu cuenta de {{appName}}. A partir de ahora necesitarás un código cada vez que inicies sesión."}},` +
				`{"id":"s3","type":"text","props":{"text":"Guarda tus códigos de respaldo en un lugar seguro: {{backupCodes}}"}},` +
				`{"id":"s4","type":"text","props":{"text":"¿No fuiste tú? Contacta con soporte de inmediato."}},` +
				`{"id":"s5","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","backupCodes","year"]`,
		},
		{
			key: "2fa-disabled", category: "security", name: "2FA desactivado",
			description: "Confirmación de que se desactivó la verificación en dos pasos.",
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"{{appName}}"}},` +
				`{"id":"s2","type":"text","props":{"text":"Hola {{firstName}}, has desactivado la verificación en dos pasos (2FA) en tu cuenta de {{appName}}. Tu cuenta es ahora menos segura; te recomendamos volver a activarla."}},` +
				`{"id":"s3","type":"text","props":{"text":"Cambio realizado el {{changedAt}} desde la IP {{ipAddress}}."}},` +
				`{"id":"s4","type":"button","props":{"text":"Reactivar 2FA","url":"{{reactivateUrl}}"}},` +
				`{"id":"s5","type":"text","props":{"text":"¿No fuiste tú? Tu cuenta podría estar comprometida: cambia tu contraseña y contacta con soporte."}},` +
				`{"id":"s6","type":"footer","props":{"text":"© {{year}} {{appName}}"}}]}`,
			suggested: `["appName","firstName","changedAt","ipAddress","reactivateUrl","year"]`,
		},
		{
			key: "contact-form", category: "system", name: "Formulario de contacto",
			description: "Aviso de un nuevo mensaje de contacto (destinatario anónimo).",
			// Solo variables que el caller SIEMPRE manda (contact_http.go del
			// RMS: name/email/message/locale) — una {{var}} sin valor = 422.
			structure: theme +
				`{"id":"s1","type":"header","props":{"title":"Nuevo mensaje de contacto"}},` +
				`{"id":"s2","type":"text","props":{"text":"Ha llegado un mensaje desde el formulario de la web.","color":"#666666"}},` +
				`{"id":"s3","type":"divider","props":{}},` +
				`{"id":"s4","type":"text","props":{"text":"De: {{name}}\nEmail: {{email}}\nIdioma: {{locale}}","fontSize":14}},` +
				`{"id":"s5","type":"divider","props":{}},` +
				`{"id":"s6","type":"text","props":{"text":"{{message}}"}},` +
				`{"id":"s7","type":"spacer","props":{}},` +
				`{"id":"s8","type":"button","props":{"text":"Responder a {{name}}","url":"mailto:{{email}}"}},` +
				`{"id":"s9","type":"footer","props":{"text":"Formulario de contacto de divergtech.com — responde directamente a este mensaje para contactar con el remitente."}}]}`,
			suggested: `["name","email","message","locale"]`,
		},
	}

	// DO UPDATE: la librería de base es autoritativa; re-sembrar refresca el
	// contenido de las plantillas existentes (no solo inserta las nuevas).
	const q = `
INSERT INTO base_templates (key, channel, category, name, description, structure, suggested_variables)
VALUES ($1, 'EMAIL', $2, $3, $4, $5::jsonb, $6::jsonb)
ON CONFLICT (key) DO UPDATE SET
    category = EXCLUDED.category,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    structure = EXCLUDED.structure,
    suggested_variables = EXCLUDED.suggested_variables,
    updated_at = now()`
	logo := brandLogoDataURI()
	for _, it := range items {
		// Inyecta el logo de marca en la cabecera de cada plantilla.
		structure := strings.ReplaceAll(it.structure, `"type":"header","props":{`, `"type":"header","props":{"logoUrl":"`+logoToken+`",`)
		structure = strings.ReplaceAll(structure, logoToken, logo)
		if _, err := pool.Exec(ctx, q, it.key, it.category, it.name, it.description, structure, it.suggested); err != nil {
			return err
		}
	}
	return nil
}

// htmlBaseEntry describe una plantilla base en modo HTML (índice embebido).
type htmlBaseEntry struct {
	Key         string   `json:"key"`
	File        string   `json:"file"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	Suggested   []string `json:"suggested"`
}

// seedHTMLBaseTemplates siembra las 12 plantillas por defecto en modo 'html'. El
// HTML embebido se SANEA con el saneador de producción antes de guardarlo.
func seedHTMLBaseTemplates(ctx context.Context, pool *pgxpool.Pool) error {
	raw, err := htmlBasesFS.ReadFile("htmlbases/index.json")
	if err != nil {
		return fmt.Errorf("índice: %w", err)
	}
	var items []htmlBaseEntry
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("índice inválido: %w", err)
	}

	const q = `
INSERT INTO base_templates (key, channel, category, name, description, structure, suggested_variables, kind, body, subject)
VALUES ($1, 'EMAIL', $2, $3, $4, '{}'::jsonb, $5::jsonb, 'html', $6, $7)
ON CONFLICT (key) DO UPDATE SET
    category = EXCLUDED.category,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    suggested_variables = EXCLUDED.suggested_variables,
    kind = EXCLUDED.kind,
    body = EXCLUDED.body,
    subject = EXCLUDED.subject,
    updated_at = now()`

	logo := brandLogoDataURI()
	for _, it := range items {
		htmlBytes, err := htmlBasesFS.ReadFile("htmlbases/" + it.File)
		if err != nil {
			return fmt.Errorf("leyendo %s: %w", it.File, err)
		}
		htmlStr := strings.ReplaceAll(string(htmlBytes), logoToken, logo)
		sanitized, err := template.SanitizeEmailHTML(htmlStr)
		if err != nil {
			return fmt.Errorf("saneando %s: %w", it.Key, err)
		}
		suggested, _ := json.Marshal(it.Suggested)
		if _, err := pool.Exec(ctx, q, it.Key, it.Category, it.Name, it.Description, suggested, sanitized, it.Subject); err != nil {
			return fmt.Errorf("insertando %s: %w", it.Key, err)
		}
	}
	return nil
}

// seedSuperAdmin crea el administrador de plataforma (idempotente por email).
func seedSuperAdmin(ctx context.Context, pool *pgxpool.Pool, email, password string) error {
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return err
	}
	const q = `
INSERT INTO admin_users (email, password_hash, role)
VALUES ($1, $2, 'SUPER_ADMIN')
ON CONFLICT (email) DO NOTHING`
	_, err = pool.Exec(ctx, q, email, hash)
	return err
}

// seedDemoApp crea una aplicación demo con un Template "welcome" ya compilado.
func seedDemoApp(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	const insertApp = `
INSERT INTO applications (name, slug) VALUES ('Demo', 'demo')
ON CONFLICT (slug) DO NOTHING`
	if _, err := pool.Exec(ctx, insertApp); err != nil {
		return "", err
	}

	var appID string
	if err := pool.QueryRow(ctx, `SELECT id FROM applications WHERE slug = 'demo'`).Scan(&appID); err != nil {
		return "", err
	}

	const body = `<!doctype html><html><body style="font-family:Arial,sans-serif">` +
		`<h1>{{appName}}</h1><p>Hola {{firstName}}, te damos la bienvenida.</p>` +
		`<p><a href="{{activationUrl}}" style="padding:10px 16px;background:#0B5;color:#fff;text-decoration:none">Activar cuenta</a></p>` +
		`<hr><small>© {{year}} {{appName}}</small></body></html>`
	const structure = `{"sections":[{"id":"s1","type":"header","props":{"title":"{{appName}}"}}]}`
	const requiredVars = `{"type":"object","required":["firstName","activationUrl","appName"],` +
		`"properties":{"firstName":{"type":"string"},"activationUrl":{"type":"string"},` +
		`"appName":{"type":"string"},"year":{"type":"string"}}}`

	const insertTpl = `
INSERT INTO templates (application_id, key, channel, name, subject, structure, body, required_variables, locale, version)
VALUES ($1, 'welcome', 'EMAIL', 'Bienvenida', 'Bienvenido a {{appName}}', $2::jsonb, $3, $4::jsonb, 'en', 1)
ON CONFLICT (application_id, key, locale, version) DO NOTHING`
	if _, err := pool.Exec(ctx, insertTpl, appID, structure, body, requiredVars); err != nil {
		return "", err
	}
	return appID, nil
}
