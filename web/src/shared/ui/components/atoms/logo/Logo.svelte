<script lang="ts">
	// El muelle del diseño: una tabla sobre tres pilotes. Decorativo; el texto o el
	// `aria-label` del link que lo envuelve dice qué es.
	let { size = 22 }: { size?: number } = $props()
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
	<rect x="2" y="6" width="20" height="3.2" rx="0.6" />
	<rect x="4.5" y="9" width="2.6" height="10" />
	<rect x="10.7" y="9" width="2.6" height="10" />
	<rect x="16.9" y="9" width="2.6" height="10" />
</svg>
