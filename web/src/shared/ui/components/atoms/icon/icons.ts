// Los íconos del diseño, a trazo (24×24, `stroke-width` 2, puntas redondeadas). `fill`
// marca los que van rellenos (los tres puntos de "Opciones").
export interface IconShape {
	readonly paths: readonly string[]
	readonly circles?: readonly (readonly [cx: number, cy: number, r: number])[]
	readonly rects?: readonly (readonly [x: number, y: number, w: number, h: number, rx: number])[]
	readonly fill?: boolean
}

export const ICONS = {
	plus: { paths: ['M12 5v14M5 12h14'] },
	search: { paths: ['m20 20-3.5-3.5'], circles: [[11, 11, 7]] },
	more: {
		paths: [],
		circles: [
			[5, 12, 1.6],
			[12, 12, 1.6],
			[19, 12, 1.6],
		],
		fill: true,
	},
	chevronRight: { paths: ['m9 6 6 6-6 6'] },
	chevronDown: { paths: ['m6 9 6 6 6-6'] },
	chevronUpDown: { paths: ['m7 15 5 5 5-5M7 9l5-5 5 5'] },
	arrowLeft: { paths: ['M19 12H5M11 18l-6-6 6-6'] },
	arrowUpRight: { paths: ['M7 17 17 7M9 7h8v8'] },
	copy: {
		paths: ['M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2'],
		rects: [[8, 8, 12, 12, 2]],
	},
	check: { paths: ['m5 12 5 5 9-10'] },
	x: { paths: ['M18 6 6 18M6 6l12 12'] },
	lock: { paths: ['M8 11V8a4 4 0 0 1 8 0v3'], rects: [[5, 11, 14, 9, 2]] },
	download: { paths: ['M12 4v11M7 10l5 5 5-5M5 20h14'] },
	upload: { paths: ['M12 20V9M7 14l5-5 5 5M5 4h14'] },
	refresh: { paths: ['M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7'] },
	trash: { paths: ['M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3'] },
	logOut: { paths: ['M15 4h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4M10 17l5-5-5-5M15 12H4'] },
	shield: { paths: ['M12 3 5 6v6c0 4.5 3 7.5 7 9 4-1.5 7-4.5 7-9V6l-7-3Z', 'm9 12 2 2 4-4'] },
	globe: {
		paths: ['M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18'],
		circles: [[12, 12, 9]],
	},
	alert: { paths: ['M12 8v5M12 16.5v.5'], circles: [[12, 12, 9]] },
	clock: { paths: ['M12 7v5l3 2'], circles: [[12, 12, 9]] },
	external: {
		paths: ['M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5'],
	},
} as const satisfies Record<string, IconShape>

export type IconName = keyof typeof ICONS
