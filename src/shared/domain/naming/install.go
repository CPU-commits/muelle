package naming

// LabelSwarmService la pone Swarm en cada contenedor de una tarea, con el
// nombre del servicio. muelle settings reset-panel-ips la lee en su propio
// contenedor para saber qué servicio reiniciar (contrato §5.1 y §5.4).
const LabelSwarmService = "com.docker.swarm.service.name"

// InstallProbeContainer es el contenedor temporal con que install prueba
// los puertos del proxy (contrato §5.4); install lo borra siempre.
const InstallProbeContainer = "muelle-install-probe"

// InstallNames son los nombres de lo que crea o mira install. Con el
// prefijo "muelle" son los fijos de este paquete; otro prefijo es solo
// para los tests de integración (--name-prefix, contrato §5.1).
type InstallNames struct {
	Stack, Service, PanelNetwork, DataVolume, CaddyAdminVolume     string
	MasterKeySecret, InitCloudflareTokenSecret, InitRegistrySecret string
	ProbeContainer, CaddyContainer                                 string
}

// InstallNamesFor arma los nombres de install con el prefijo p en vez de
// "muelle": <p>, <p>_muelle, <p>_panel… Con p = MuelleStack son exactamente
// las constantes (TestInstallNamesForMuelle lo verifica). No valida p.
func InstallNamesFor(p string) InstallNames {
	return InstallNames{
		Stack:                     p,
		Service:                   p + sep + "muelle",
		PanelNetwork:              p + sep + "panel",
		DataVolume:                p + sep + "data",
		CaddyAdminVolume:          p + sep + "caddy_admin",
		MasterKeySecret:           p + sep + "master_key",
		InitCloudflareTokenSecret: p + sep + "init_cloudflare_token",
		InitRegistrySecret:        p + sep + "init_registry",
		ProbeContainer:            p + "-install-probe",
		CaddyContainer:            p + "-caddy",
	}
}
