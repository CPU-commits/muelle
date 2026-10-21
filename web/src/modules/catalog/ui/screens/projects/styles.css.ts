import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Proyectos (`Main.dc.html`).

// El buscador ocupa hasta 340 px y se achica antes de que el botón salte de línea.
export const toolbar = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'flex-end',
	gap: theme.space[8],
	minWidth: 0,
})

export const search = style({
	flex: '1 1 340px',
	width: '340px',
	minWidth: '200px',
})

export const projects = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[28],
})

export const project = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	scrollMarginTop: theme.space[24],
})

export const projectHead = style({
	display: 'flex',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: theme.space[12],
})

export const projectTitle = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[4]} ${theme.space[10]}`,
	minWidth: 0,
})

export const initial = style({
	display: 'grid',
	placeItems: 'center',
	flex: 'none',
	width: theme.space[24],
	height: theme.space[24],
	borderRadius: theme.radius.md,
	border: `1px solid ${theme.colors.borders.control}`,
	backgroundColor: theme.colors.background.raised,
	color: theme.colors.text.secondary,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[12],
})

export const projectName = style({
	margin: 0,
	fontSize: theme.typography.fontSize[15],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
	overflowWrap: 'anywhere',
})

export const meta = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const card = style({
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	backgroundColor: theme.colors.background.surface,
	overflow: 'hidden',
})

export const stageRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[4]} ${theme.space[12]}`,
	padding: `${theme.space[6]} ${theme.space[8]} ${theme.space[6]} ${theme.space[16]}`,
	backgroundColor: theme.colors.background.sunken,
	borderBottom: `1px solid ${theme.colors.borders.subtle}`,
})

export const stageRowNext = style({
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
})

export const stageTitle = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[2]} ${theme.space[10]}`,
	minWidth: 0,
	fontSize: theme.typography.fontSize[13],
})

export const stageName = style({
	margin: 0,
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
})

export const stageActions = style({
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[2],
})

export const variablesLink = style({
	display: 'inline-flex',
	alignItems: 'center',
	gap: theme.space[4],
	height: theme.control.xs,
	paddingInline: theme.space[8],
	borderRadius: theme.radius.md,
	color: theme.colors.text.secondary,
	fontSize: theme.typography.fontSize[13],
	textDecoration: 'none',
	transition: `background-color ${theme.transition.fast}`,
	selectors: {
		'&:hover': {
			backgroundColor: theme.colors.background.raised,
			color: theme.colors.text.primary,
		},
	},
})

export const apps = style({
	listStyle: 'none',
	margin: 0,
	padding: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: '1px',
	backgroundColor: theme.colors.background.divider,
})

export const emptyRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: theme.space[8],
	padding: `${theme.space[10]} ${theme.space[12]} ${theme.space[10]} ${theme.space[16]}`,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
