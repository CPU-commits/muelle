import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const stack = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
	minWidth: 0,
})

// El recuadro de estado del token de Cloudflare.
export const status = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[8]} ${theme.space[16]}`,
	maxWidth: '560px',
	boxSizing: 'border-box',
	padding: `${theme.space[12]} ${theme.space[14]}`,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.background.page,
})

export const statusLabel = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
})

export const statusMeta = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const fields = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fit, minmax(min(200px, 100%), 1fr))',
	gap: theme.space[16],
	maxWidth: '560px',
})

// El aviso de la dirección nueva del panel, con el comando de MUELLE_PANEL_ORIGIN abajo.
export const moved = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[12],
	maxWidth: '720px',
})

export const portFields = style({
	display: 'grid',
	gridTemplateColumns: 'repeat(auto-fit, minmax(min(140px, 100%), 1fr))',
	gap: theme.space[16],
	maxWidth: '360px',
})

// "Dominio del panel": un valor de solo lectura con la misma forma que un campo.
export const readonlyField = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
})

export const readonlyLabel = style({
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.text.secondary,
})

export const readonlyValue = style({
	display: 'flex',
	alignItems: 'center',
	minHeight: theme.control.md,
	overflowWrap: 'anywhere',
})

export const warning = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.status.warnText,
})
