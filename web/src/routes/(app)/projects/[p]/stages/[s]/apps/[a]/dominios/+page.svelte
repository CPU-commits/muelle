<script lang="ts">
	import { DomainsScreen } from '@/modules/ingress'
	import type { PageProps } from './$types'

	// Los destinos posibles (`targets`, "servicio:puerto") salen del catálogo: los conecta F2.
	let { params }: PageProps = $props()
</script>

<DomainsScreen project={params.p} stage={params.s} app={params.a} />
