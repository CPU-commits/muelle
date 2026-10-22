<script lang="ts">
	import { DomainsScreen } from '@/modules/ingress'
	import type { PageProps } from './$types'

	// Los destinos posibles ("servicio:puerto") los pide DomainsScreen al catálogo (§2.12).
	let { params }: PageProps = $props()
</script>

<DomainsScreen project={params.p} stage={params.s} app={params.a} />
