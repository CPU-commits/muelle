package entity

// Claves de settings del registro DNS del panel (contrato §3.6). Las escribe
// solo ingress; GET /settings no las muestra y PATCH /settings no las acepta.
const (
	// SettingPanelCFZoneID es la zona de panel_domain, si cae en una visible
	// para el token; decide el TLS del panel.
	SettingPanelCFZoneID = "panel_cf_zone_id"
	// SettingPanelCFRecordID y SettingPanelCFRecordIDAAAA son los registros A
	// y AAAA del panel que creó muelle; no están si no creó ninguno.
	SettingPanelCFRecordID     = "panel_cf_record_id"
	SettingPanelCFRecordIDAAAA = "panel_cf_record_id_aaaa"
)
