import { style } from '@vanilla-extract/css'
import { media } from '@/shared/ui/themes/media'
import { theme } from '@/shared/ui/themes/theme.css'

// El panel "Importar .env" de App-Variables.dc.html: cuerpo con padding y pie más oscuro.

export const panel = style({
	border: `1px solid ${theme.colors.borders.control}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
	overflow: 'hidden',
	minWidth: 0,
})

export const body = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[14],
	padding: theme.space[16],
	'@media': {
		[media.sm]: { padding: `${theme.space[20]} ${theme.space[24]}` },
	},
})

export const title = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const textarea = style({
	backgroundColor: theme.colors.background.page,
	lineHeight: 1.6,
	resize: 'vertical',
})

export const hint = style({
	marginTop: `calc(-1 * ${theme.space[6]})`,
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const options = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[8]} ${theme.space[24]}`,
})

// El contenedor de los problemas y avisos (una región viva: siempre en el árbol de
// accesibilidad, nunca `display: none`). Vacío, se come el espacio del `gap` del cuerpo.
export const live = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	selectors: { '&:empty': { marginBottom: `calc(-1 * ${theme.space[14]})` } },
})

export const issues = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[6],
	padding: `${theme.space[10]} ${theme.space[14]}`,
	border: `1px solid ${theme.colors.status.errorBorder}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.status.errorSurface,
	fontSize: theme.typography.fontSize[13],
})

export const issuesTitle = style({
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.status.errorText,
})

export const issueList = style({
	margin: 0,
	paddingLeft: theme.space[16],
	maxHeight: '160px',
	overflowY: 'auto',
	color: theme.colors.text.secondary,
	overflowWrap: 'anywhere',
})

export const notes = style({
	margin: 0,
	paddingLeft: theme.space[16],
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.status.warnText,
	overflowWrap: 'anywhere',
})

export const preview = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	paddingTop: theme.space[14],
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
})

export const previewTitle = style({
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.semibold,
})

export const groups = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	margin: 0,
	maxHeight: '280px',
	overflowY: 'auto',
})

export const group = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'baseline',
	gap: `${theme.space[4]} ${theme.space[12]}`,
})

export const groupLabel = style({
	flex: '0 0 170px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const groupKeys = style({
	flex: '1 1 280px',
	minWidth: 0,
	display: 'flex',
	flexWrap: 'wrap',
	gap: `${theme.space[4]} ${theme.space[12]}`,
	margin: 0,
})

export const key = style({
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
	overflowWrap: 'anywhere',
})

export const keyRemoved = style({
	color: theme.colors.status.errorText,
	textDecoration: 'line-through',
})

export const changeItem = style({
	display: 'inline-flex',
	flexWrap: 'wrap',
	alignItems: 'baseline',
	gap: theme.space[6],
	minWidth: 0,
})

export const reason = style({
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const groupHint = style({
	flexBasis: '100%',
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const muted = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const footer = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
	padding: `${theme.space[12]} ${theme.space[16]}`,
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.sunken,
	'@media': {
		[media.sm]: { paddingInline: theme.space[24] },
	},
})

export const summary = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const actions = style({
	display: 'flex',
	gap: theme.space[8],
})
