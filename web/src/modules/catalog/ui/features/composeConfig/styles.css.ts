import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const page = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
})

export const actions = style({
	display: 'flex',
	flexWrap: 'wrap',
	gap: theme.space[8],
})

// El editor a la izquierda (2) y el aside a la derecha (1); en un teléfono, uno abajo
// del otro.
export const layout = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-start',
	gap: theme.space[16],
})

export const main = style({
	flex: '2 1 620px',
	minWidth: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[12],
})

export const aside = style({
	flex: '1 1 300px',
	minWidth: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[16],
})

// La lista de errores ("No se guardó: 2 errores").
export const errors = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[8],
	padding: `${theme.space[12]} ${theme.space[16]}`,
	border: `1px solid ${theme.colors.status.errorBorder}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.status.errorSunken,
})

export const errorsTitle = style({
	margin: 0,
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[10],
	fontWeight: theme.typography.fontWeight.medium,
})

export const errorsList = style({
	margin: 0,
	padding: `0 0 0 ${theme.space[16]}`,
	listStyle: 'none',
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[4],
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const lineLink = style({
	padding: 0,
	border: 0,
	background: 'none',
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: 'inherit',
	color: theme.colors.status.errorText,
	cursor: 'pointer',
	textDecoration: 'underline',
	textDecorationColor: 'transparent',
	selectors: {
		'&:hover': { textDecorationColor: 'currentColor' },
	},
})

export const editor = style({
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.page,
	overflow: 'hidden',
})

export const editorBar = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[4]} ${theme.space[12]}`,
	minHeight: theme.control.lg,
	padding: `${theme.space[4]} ${theme.space[12]} ${theme.space[4]} ${theme.space[16]}`,
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.surface,
})

export const fileName = style({
	margin: 0,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.regular,
	color: theme.colors.text.secondary,
})

export const barEnd = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: theme.space[8],
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const hint = style({
	margin: 0,
	padding: `${theme.space[8]} ${theme.space[16]}`,
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.sunken,
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const serviceList = style({
	listStyle: 'none',
	margin: 0,
	padding: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: '1px',
	backgroundColor: theme.colors.background.divider,
})

export const service = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
	padding: `${theme.space[12]} ${theme.space[16]}`,
	backgroundColor: theme.colors.background.surface,
})

export const serviceHead = style({
	display: 'flex',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: theme.space[8],
	minWidth: 0,
})

export const serviceName = style({
	fontFamily: theme.typography.fontFamily.mono,
	fontWeight: theme.typography.fontWeight.medium,
	overflowWrap: 'anywhere',
})

export const serviceWarning = style({
	flex: 'none',
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.status.warnText,
})

export const serviceDetails = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const servicesEmpty = style({
	margin: 0,
	padding: `${theme.space[12]} ${theme.space[16]}`,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const injections = style({
	margin: 0,
	padding: `${theme.space[12]} ${theme.space[16]} ${theme.space[14]}`,
	display: 'grid',
	gridTemplateColumns: 'auto 1fr',
	gap: `${theme.space[8]} ${theme.space[16]}`,
	fontSize: theme.typography.fontSize[13],
})

export const injectionTerm = style({ color: theme.colors.text.tertiary })
export const injectionValue = style({ margin: 0, minWidth: 0, overflowWrap: 'anywhere' })

export const injectionNote = style({
	margin: 0,
	padding: `0 ${theme.space[16]} ${theme.space[14]}`,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
