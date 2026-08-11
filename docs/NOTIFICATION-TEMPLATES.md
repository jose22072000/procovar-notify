# 📧 QB Notify - Guía de Templates de Notificación

Esta guía documenta todos los templates de email disponibles y cómo enviar notificaciones usando cada uno.

## Configuración Base

**URL Base:** `http://localhost:5100/api`  
**Autenticación:** Bearer Token  
**Content-Type:** `application/json`

```bash
# Variables de entorno recomendadas
export NOTIFY_URL="http://localhost:5100/api"
export NOTIFY_TOKEN="your-bearer-token"
```

---

## 📋 Templates Disponibles

| Template | Descripción | Uso Principal |
|----------|-------------|---------------|
| `email-verification` | Verificación de correo | Registro de usuario |
| `welcome` | Bienvenida | Post-verificación |
| `forgot-password` | Recuperar contraseña | Olvidé mi contraseña |
| `password-reset-success` | Contraseña actualizada | Post-cambio de contraseña |
| `otp-verification` | Código OTP | 2FA / Login |
| `invitation` | Invitación | Invitar usuarios |
| `magic-link` | Enlace mágico | Login sin contraseña |
| `account-deletion` | Eliminar cuenta | Confirmar eliminación |
| `email-change` | Cambio de email | Verificar nuevo email |
| `security-alert` | Alerta de seguridad | Actividad sospechosa |
| `two-factor-enabled` | 2FA activado | Confirmación 2FA |
| `two-factor-disabled` | 2FA desactivado | Confirmación desactivación |

---

## 1. 📨 Email Verification (Verificación de Correo)

Envía un email de verificación cuando un usuario se registra.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `expirationTime` | string | ✅ | Tiempo de expiración (ej: "24 horas") |
| `verificationCode` | string | ❌ | Código de verificación |
| `verificationUrl` | string | ❌ | URL de verificación |
| `userName` | string | ❌ | Nombre del usuario |
| `logoUrl` | string | ❌ | URL del logo |
| `primaryColor` | string | ❌ | Color primario (hex) |
| `supportEmail` | string | ❌ | Email de soporte |
| `year` | string | ❌ | Año actual |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "email-verification",
    "type": "EMAIL",
    "priority": "HIGH",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "verificationCode": "847291",
      "verificationUrl": "https://miapp.com/verify?token=abc123xyz",
      "expirationTime": "24 horas",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#4F46E5",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 2. 🎉 Welcome (Bienvenida)

Email de bienvenida después del registro exitoso.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `userName` | string | ✅ | Nombre del usuario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `actionUrl` | string | ❌ | URL de acción principal |
| `actionLabel` | string | ❌ | Texto del botón |
| `features` | array | ❌ | Lista de características |
| `logoUrl` | string | ❌ | URL del logo |
| `year` | string | ❌ | Año actual |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "welcome",
    "type": "EMAIL",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "actionUrl": "https://miapp.com/dashboard",
      "actionLabel": "Ir al Dashboard",
      "features": [
        "Gestiona tus proyectos fácilmente",
        "Colabora con tu equipo en tiempo real",
        "Accede desde cualquier dispositivo"
      ],
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#10B981",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 3. 🔑 Forgot Password (Olvidé mi Contraseña)

Email para recuperación de contraseña.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `expirationTime` | string | ✅ | Tiempo de expiración |
| `resetCode` | string | ❌ | Código de recuperación |
| `resetUrl` | string | ❌ | URL de recuperación |
| `userName` | string | ❌ | Nombre del usuario |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "forgot-password",
    "type": "EMAIL",
    "priority": "HIGH",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "resetCode": "492817",
      "resetUrl": "https://miapp.com/reset-password?token=xyz789abc",
      "expirationTime": "1 hora",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#EF4444",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 4. ✅ Password Reset Success (Contraseña Actualizada)

Confirmación de que la contraseña fue cambiada exitosamente.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `changedAt` | string | ✅ | Fecha/hora del cambio |
| `userName` | string | ❌ | Nombre del usuario |
| `ipAddress` | string | ❌ | IP desde donde se cambió |
| `loginUrl` | string | ❌ | URL de login |
| `supportEmail` | string | ❌ | Email de soporte |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "password-reset-success",
    "type": "EMAIL",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "changedAt": "25 de diciembre de 2025 a las 10:30 AM",
      "ipAddress": "192.168.1.100",
      "loginUrl": "https://miapp.com/login",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#10B981",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 5. 🔢 OTP Verification (Código OTP)

Envía un código OTP para verificación de dos factores.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `otpCode` | string | ✅ | Código OTP |
| `expirationTime` | string | ✅ | Tiempo de expiración |
| `userName` | string | ❌ | Nombre del usuario |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "otp-verification",
    "type": "EMAIL",
    "priority": "URGENT",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "otpCode": "847291",
      "expirationTime": "5 minutos",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#6366F1",
      "year": "2025"
    }
  }'
```

---

## 6. 📩 Invitation (Invitación)

Invita a un usuario a unirse a la plataforma u organización.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `inviterName` | string | ✅ | Nombre de quien invita |
| `invitationUrl` | string | ✅ | URL de invitación |
| `expirationTime` | string | ✅ | Tiempo de expiración |
| `inviteeName` | string | ❌ | Nombre del invitado |
| `organizationName` | string | ❌ | Nombre de la organización |
| `role` | string | ❌ | Rol asignado |
| `message` | string | ❌ | Mensaje personalizado |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "invitation",
    "type": "EMAIL",
    "payload": {
      "email": "nuevo.usuario@ejemplo.com",
      "inviteeName": "María López",
      "inviterName": "Juan García",
      "organizationName": "Empresa ABC",
      "appName": "MiApp",
      "role": "Editor",
      "message": "¡Hola María! Te invito a colaborar en nuestro proyecto. Será genial trabajar contigo.",
      "invitationUrl": "https://miapp.com/invite/accept?token=inv123xyz",
      "expirationTime": "7 días",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#8B5CF6",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 7. 🔗 Magic Link (Enlace Mágico)

Login sin contraseña mediante enlace mágico.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `magicLinkUrl` | string | ✅ | URL del enlace mágico |
| `expirationTime` | string | ✅ | Tiempo de expiración |
| `userName` | string | ❌ | Nombre del usuario |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "magic-link",
    "type": "EMAIL",
    "priority": "HIGH",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "magicLinkUrl": "https://miapp.com/auth/magic?token=mgk789xyz123",
      "expirationTime": "15 minutos",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#0EA5E9",
      "year": "2025"
    }
  }'
```

---

## 8. 🗑️ Account Deletion (Eliminar Cuenta)

Confirmación para eliminar la cuenta del usuario.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `confirmationUrl` | string | ✅ | URL de confirmación |
| `expirationTime` | string | ✅ | Tiempo de expiración |
| `userName` | string | ❌ | Nombre del usuario |
| `additionalData` | string | ❌ | Datos adicionales que se eliminarán |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "account-deletion",
    "type": "EMAIL",
    "priority": "HIGH",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "confirmationUrl": "https://miapp.com/account/delete/confirm?token=del456abc",
      "expirationTime": "24 horas",
      "additionalData": "Todos tus archivos y documentos",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#DC2626",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 9. 📧 Email Change (Cambio de Email)

Verificación para cambiar el correo electrónico.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario (nuevo email) |
| `appName` | string | ✅ | Nombre de la aplicación |
| `currentEmail` | string | ✅ | Email actual |
| `newEmail` | string | ✅ | Nuevo email |
| `expirationTime` | string | ✅ | Tiempo de expiración |
| `verificationCode` | string | ❌ | Código de verificación |
| `confirmationUrl` | string | ❌ | URL de confirmación |
| `userName` | string | ❌ | Nombre del usuario |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "email-change",
    "type": "EMAIL",
    "priority": "HIGH",
    "payload": {
      "email": "nuevo.email@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "currentEmail": "usuario@ejemplo.com",
      "newEmail": "nuevo.email@ejemplo.com",
      "verificationCode": "583921",
      "confirmationUrl": "https://miapp.com/account/email/confirm?token=eml789xyz",
      "expirationTime": "1 hora",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#F59E0B",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 10. ⚠️ Security Alert (Alerta de Seguridad)

Notifica sobre actividad sospechosa en la cuenta.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `alertType` | string | ✅ | Tipo de alerta |
| `userName` | string | ❌ | Nombre del usuario |
| `details` | object | ❌ | Detalles del evento |
| `details.time` | string | ❌ | Fecha/hora del evento |
| `details.location` | string | ❌ | Ubicación |
| `details.device` | string | ❌ | Dispositivo |
| `details.ipAddress` | string | ❌ | Dirección IP |
| `secureAccountUrl` | string | ❌ | URL para asegurar cuenta |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "security-alert",
    "type": "EMAIL",
    "priority": "URGENT",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "alertType": "Nuevo inicio de sesión detectado",
      "details": {
        "time": "25 de diciembre de 2025 a las 03:45 AM",
        "location": "Ciudad de México, México",
        "device": "Chrome en Windows",
        "ipAddress": "203.0.113.42"
      },
      "secureAccountUrl": "https://miapp.com/account/security",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#DC2626",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 11. 🔐 Two Factor Enabled (2FA Activado)

Confirmación de activación de autenticación de dos factores.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `userName` | string | ❌ | Nombre del usuario |
| `backupCodes` | array | ❌ | Códigos de respaldo |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "two-factor-enabled",
    "type": "EMAIL",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "backupCodes": [
        "A1B2-C3D4",
        "E5F6-G7H8",
        "I9J0-K1L2",
        "M3N4-O5P6",
        "Q7R8-S9T0"
      ],
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#10B981",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 12. 🔓 Two Factor Disabled (2FA Desactivado)

Confirmación de desactivación de autenticación de dos factores.

### Payload Requerido
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| `email` | string | ✅ | Email del destinatario |
| `appName` | string | ✅ | Nombre de la aplicación |
| `changedAt` | string | ✅ | Fecha/hora del cambio |
| `userName` | string | ❌ | Nombre del usuario |
| `ipAddress` | string | ❌ | IP desde donde se desactivó |
| `reactivateUrl` | string | ❌ | URL para reactivar 2FA |
| `logoUrl` | string | ❌ | URL del logo |

### Ejemplo cURL

```bash
curl -X POST "${NOTIFY_URL}/notifications" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userId": "123e4567-e89b-12d3-a456-426614174000",
    "templateId": "two-factor-disabled",
    "type": "EMAIL",
    "priority": "HIGH",
    "payload": {
      "email": "usuario@ejemplo.com",
      "userName": "Juan García",
      "appName": "MiApp",
      "changedAt": "25 de diciembre de 2025 a las 11:00 AM",
      "ipAddress": "192.168.1.100",
      "reactivateUrl": "https://miapp.com/account/security/2fa",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#F59E0B",
      "supportEmail": "soporte@miapp.com",
      "year": "2025"
    }
  }'
```

---

## 🔄 Envío Bulk

Para enviar la misma notificación a múltiples usuarios:

```bash
curl -X POST "${NOTIFY_URL}/notifications/bulk" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  -d '{
    "userIds": [
      "123e4567-e89b-12d3-a456-426614174000",
      "223e4567-e89b-12d3-a456-426614174001",
      "323e4567-e89b-12d3-a456-426614174002"
    ],
    "templateId": "welcome",
    "type": "EMAIL",
    "payload": {
      "email": "broadcast@ejemplo.com",
      "userName": "Usuario",
      "appName": "MiApp",
      "actionUrl": "https://miapp.com/dashboard",
      "logoUrl": "https://miapp.com/logo.png",
      "primaryColor": "#4F46E5",
      "year": "2025"
    }
  }'
```

---

## 📊 Variables Comunes en Todos los Templates

Estas variables están disponibles en todos los templates:

| Variable | Descripción | Ejemplo |
|----------|-------------|---------|
| `logoUrl` | URL del logo de la aplicación | `https://miapp.com/logo.png` |
| `primaryColor` | Color primario para botones | `#4F46E5` |
| `appName` | Nombre de la aplicación | `MiApp` |
| `year` | Año actual para el footer | `2025` |
| `supportEmail` | Email de soporte | `soporte@miapp.com` |
| `unsubscribeUrl` | URL para cancelar suscripción | `https://miapp.com/unsubscribe` |

---

## 🎨 Colores Recomendados

| Propósito | Color | Hex |
|-----------|-------|-----|
| Principal / Éxito | Verde | `#10B981` |
| Información | Azul | `#0EA5E9` |
| Advertencia | Amarillo | `#F59E0B` |
| Error / Peligro | Rojo | `#DC2626` |
| Acción | Índigo | `#4F46E5` |
| Secundario | Púrpura | `#8B5CF6` |

---

## 📝 Notas Importantes

1. **El campo `email` es obligatorio** en el payload para todos los templates de tipo EMAIL.
2. **`userId`** debe ser un UUID válido.
3. **`templateId`** debe coincidir exactamente con el nombre del template.
4. **Prioridades disponibles**: `LOW`, `NORMAL`, `HIGH`, `URGENT`
5. **Tipos disponibles**: `EMAIL`, `SMS`, `PUSH`, `IN_APP`

---

## 🔍 Verificar Estado de Notificación

```bash
# Obtener notificación por ID
curl -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  "${NOTIFY_URL}/notifications/{notification-id}"

# Listar notificaciones de un usuario
curl -H "Authorization: Bearer ${NOTIFY_TOKEN}" \
  "${NOTIFY_URL}/notifications/user/{user-id}?page=1&limit=10"
```
