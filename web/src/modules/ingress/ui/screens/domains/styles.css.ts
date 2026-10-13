import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const page = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
	minWidth: 0,
})

// "api.tudominio.cl  [api:3000]  Agregar": en una fila que envuelve.
export const form = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
})

export const formRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-start',
	gap: theme.space[8],
})

export const hostnameField = style({ flex: '3 1 280px', minWidth: 0 })

export const targetField = style({ flex: '1 1 160px', minWidth: 0 })

export const fieldError = style({
	marginTop: theme.space[6],
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.status.errorText,
})

export const list = style({
	listStyle: 'none',
	margin: 0,
	padding: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[12],
})

export const footnote = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const editForm = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})

/** El switch de Proxied del formulario, con su texto al lado (el nombre del control). */
export const proxiedField = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[8],
	minHeight: theme.control.md,
	color: theme.colors.text.secondary,
	fontSize: theme.typography.fontSize[13],
	whiteSpace: 'nowrap',
	cursor: 'pointer',
})
