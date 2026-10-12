import { keyframes, style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const page = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[24],
	minWidth: 0,
})

// Historial y log lado a lado; en un teléfono, uno abajo del otro.
export const layout = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-start',
	gap: theme.space[16],
})

const panel = style({
	minWidth: 0,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
	overflow: 'hidden',
})

export const history = style([panel, { flex: '1 1 340px' }])

export const historyList = style({
	listStyle: 'none',
	margin: 0,
	padding: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: '1px',
	backgroundColor: theme.colors.background.divider,
})

export const row = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[4],
	width: '100%',
	padding: `${theme.space[12]} ${theme.space[16]}`,
	border: 0,
	backgroundColor: theme.colors.background.surface,
	color: theme.colors.text.primary,
	textAlign: 'left',
	cursor: 'pointer',
	transition: `background-color ${theme.transition.fast}`,
	selectors: {
		'&:hover': { backgroundColor: theme.colors.background.hover },
		'&:focus-visible': { outlineOffset: '-2px' },
		'&[aria-pressed="true"]': { backgroundColor: theme.colors.background.raised },
	},
})

export const rowTop = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[10],
	width: '100%',
	minWidth: 0,
})

export const rowId = style({
	fontFamily: theme.typography.fontFamily.mono,
	fontWeight: theme.typography.fontWeight.medium,
})

export const rowLabel = style({
	fontSize: theme.typography.fontSize[13],
})

export const rowWhen = style({
	marginLeft: 'auto',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
	whiteSpace: 'nowrap',
})

export const rowBottom = style({
	paddingLeft: '18px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
	overflow: 'hidden',
	textOverflow: 'ellipsis',
	whiteSpace: 'nowrap',
})

export const monoTag = style({
	fontFamily: theme.typography.fontFamily.mono,
})

export const logPanel = style([panel, { flex: '2 1 560px' }])

export const logHeader = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
	padding: `${theme.space[12]} ${theme.space[12]} ${theme.space[12]} ${theme.space[20]}`,
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
})

export const logMeta = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[6]} ${theme.space[12]}`,
	minWidth: 0,
})

export const logTitle = style({
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[14],
	fontWeight: theme.typography.fontWeight.medium,
})

export const logSub = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

// "La versión nueva no pasó el healthcheck": la franja roja arriba del log.
export const failure = style({
	display: 'flex',
	alignItems: 'flex-start',
	gap: theme.space[10],
	padding: `${theme.space[12]} ${theme.space[20]}`,
	borderBottom: `1px solid ${theme.colors.status.errorDivider}`,
	backgroundColor: theme.colors.status.errorSurface,
})

export const failureDot = style({ marginTop: theme.space[6] })

export const failureText = style({
	fontWeight: theme.typography.fontWeight.medium,
	overflowWrap: 'anywhere',
})

export const notice = style({
	padding: `${theme.space[8]} ${theme.space[20]}`,
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
	backgroundColor: theme.colors.background.sunken,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const lines = style({
	listStyle: 'none',
	margin: 0,
	padding: `${theme.space[12]} 0 ${theme.space[16]}`,
})

export const line = style({
	display: 'flex',
	gap: theme.space[16],
	padding: `0 ${theme.space[20]}`,
})

export const time = style({
	flex: 'none',
	minWidth: '8ch',
	color: theme.colors.text.tertiary,
	fontVariantNumeric: 'tabular-nums',
})

export const text = style({
	flex: '1 1 auto',
	minWidth: 0,
	whiteSpace: 'pre-wrap',
	wordBreak: 'break-word',
})

export const tones = styleVariants({
	normal: { color: theme.colors.text.primary },
	command: { color: theme.colors.text.secondary },
	ok: { color: theme.colors.status.ok },
	warn: { color: theme.colors.status.warnText },
	error: { color: theme.colors.status.errorText },
})

const blink = keyframes({ '50%': { opacity: 0 } })

export const cursor = style({
	color: theme.colors.text.secondary,
	animation: `${blink} 1s steps(1) infinite`,
	'@media': {
		'(prefers-reduced-motion: reduce)': { animation: 'none' },
	},
})

export const placeholder = style({
	padding: `${theme.space[12]} ${theme.space[20]}`,
	color: theme.colors.text.tertiary,
	fontFamily: theme.typography.fontFamily.sans,
	fontSize: theme.typography.fontSize[13],
})

export const logErrorSlot = style({
	padding: theme.space[16],
	fontFamily: theme.typography.fontFamily.sans,
})
