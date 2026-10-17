<script lang="ts">
	import { ShellHeader } from '@/shared/ui'
	import { StageBreadcrumb } from '@/modules/catalog'
	import { StageVariablesScreen } from '@/modules/secrets'
	import type { PageProps } from './$types'

	let { params }: PageProps = $props()
</script>

<!-- Los selectores de proyecto y stage, como en una app: llevan a las variables del otro.
     El snippet se pinta en el header del AppShell y, al salir de la ruta, puede verse una
     vez con los params de la ruta nueva (sin `p` ni `s`): StageBreadcrumb no pide el árbol
     ni dibuja nada sin los dos (ver "Los snippets de <ShellHeader>" en web/CLAUDE.md). -->
<ShellHeader>
	{#snippet breadcrumb()}
		<StageBreadcrumb project={params.p ?? ''} stage={params.s ?? ''} />
	{/snippet}
</ShellHeader>

<!-- El `key`: al cambiar de stage la pantalla arranca de cero (sin un .env a medio importar). -->
{#key `${params.p}/${params.s}`}
	<StageVariablesScreen project={params.p} stage={params.s} />
{/key}
