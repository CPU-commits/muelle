<script lang="ts">
	import { AppVariablesScreen } from '@/modules/secrets'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<!-- El `key`: al cambiar de app (los selectores del breadcrumb) la pantalla arranca de cero,
     sin el texto de un .env a medio importar ni un formulario de la app anterior. -->
{#key `${params.p}/${params.s}/${params.a}`}
	<AppVariablesScreen project={params.p} stage={params.s} app={params.a} />
{/key}
