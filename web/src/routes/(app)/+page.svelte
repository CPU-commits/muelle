<script lang="ts">
	import { type AppRowDeployFactory, ProjectsScreen } from '@/modules/catalog'
	import { AppDeployState } from '@/modules/deploy'

	// Proyectos con la Fase 2: el estado, la imagen que corre y el último deploy de cada app
	// salen de `deploy`. Cada fila crea su `AppDeployState` al montarse y pide una vez,
	// recién cuando se ve (contrato §2.8), con el cache de `appStatusQuery` y `deploysQuery`
	// que comparten el Resumen y Deploys. Si la Fase 2 no está o falla, la fila dice "—".
	const appDeploy: AppRowDeployFactory = (app, enabled) =>
		new AppDeployState(() => {
			const { project, stage, slug } = app()
			return { project, stage, app: slug }
		}, enabled)
</script>

<ProjectsScreen {appDeploy} />
