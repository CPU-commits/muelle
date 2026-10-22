<script lang="ts">
	// El muelle: una tabla sobre dos arcos, que dibujan la "m", y el agua abajo. Decorativo; el
	// texto o el `aria-label` del link que lo envuelve dice qué es.
	let { size = 22 }: { size?: number } = $props()
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
	<path
		fill-rule="evenodd"
		d="M1.5 2.8h21v2.9H21v11.1H3V5.7H1.5zM5.6 16.8v-6.55a2.55 2.55 0 0 1 5.1 0v6.55zM13.3 16.8v-6.55a2.55 2.55 0 0 1 5.1 0v6.55z"
	/>
	<path
		d="M2.4 20.4c1.3 0 1.7-1.4 3.2-1.4s1.9 1.4 3.2 1.4 1.7-1.4 3.2-1.4 1.9 1.4 3.2 1.4 1.7-1.4 3.2-1.4 1.9 1.4 3.2 1.4"
		fill="none"
		stroke="currentColor"
		stroke-width="1.7"
		stroke-linecap="round"
		stroke-linejoin="round"
	/>
</svg>
