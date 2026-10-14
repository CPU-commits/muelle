<script lang="ts">
	import { setQueryClientContext } from '@tanstack/svelte-query'
	import { createQueryClient } from '@/shared/data/query'
	import { untrack } from 'svelte'
	import type { EnvOwner } from '../../../../domain'
	import { EnvEditorViewModel } from '../../envEditor'
	import { EnvImportPanel, EnvImportViewModel } from '..'

	// El panel "Importar .env" de verdad, con el botón que lo abre, como en la pantalla:
	// para ver qué queda vivo en memoria después de cerrarlo.
	const props: { owner: EnvOwner } = $props()

	const importer = untrack(() => {
		// Las mismas opciones que la app, sin reintentos.
		setQueryClientContext(createQueryClient({ retry: false }))
		return new EnvImportViewModel(new EnvEditorViewModel(() => props.owner))
	})
</script>

<main>
	<h1>Variables</h1>
	<button type="button" onclick={importer.toggle}>Importar .env</button>
	<EnvImportPanel vm={importer} id="import" />
</main>
