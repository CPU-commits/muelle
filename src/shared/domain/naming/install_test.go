package naming

import "testing"

// Con el prefijo "muelle", install usa exactamente los nombres fijos
// (contrato §5.1).
func TestInstallNamesForMuelle(t *testing.T) {
	want := InstallNames{
		Stack:                     MuelleStack,
		Service:                   MuelleService,
		PanelNetwork:              PanelNetwork,
		DataVolume:                DataVolume,
		CaddyAdminVolume:          CaddyAdminVolume,
		MasterKeySecret:           MasterKeySecret,
		InitCloudflareTokenSecret: InitCloudflareTokenSecret,
		InitRegistrySecret:        InitRegistrySecret,
		ProbeContainer:            InstallProbeContainer,
		CaddyContainer:            CaddyContainer,
	}
	if got := InstallNamesFor(MuelleStack); got != want {
		t.Fatalf("InstallNamesFor(muelle) =\n%+v\nesperaba\n%+v", got, want)
	}
}

// Otro prefijo (--name-prefix, solo para los tests de integración) cambia el
// "muelle" de todos los nombres.
func TestInstallNamesForPrefix(t *testing.T) {
	want := InstallNames{
		Stack:                     "it5abc",
		Service:                   "it5abc_muelle",
		PanelNetwork:              "it5abc_panel",
		DataVolume:                "it5abc_data",
		CaddyAdminVolume:          "it5abc_caddy_admin",
		MasterKeySecret:           "it5abc_master_key",
		InitCloudflareTokenSecret: "it5abc_init_cloudflare_token",
		InitRegistrySecret:        "it5abc_init_registry",
		ProbeContainer:            "it5abc-install-probe",
		CaddyContainer:            "it5abc-caddy",
	}
	if got := InstallNamesFor("it5abc"); got != want {
		t.Fatalf("InstallNamesFor(it5abc) =\n%+v\nesperaba\n%+v", got, want)
	}
}
