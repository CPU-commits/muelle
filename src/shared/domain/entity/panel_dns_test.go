package entity

import (
	"slices"
	"testing"
)

// Las claves del registro DNS del panel son las del contrato §3.6, y no son
// ajustes del proxy: admin nunca las lee ni las escribe en /settings.
func TestPanelDNSKeys(t *testing.T) {
	for got, want := range map[string]string{
		SettingPanelCFZoneID:       "panel_cf_zone_id",
		SettingPanelCFRecordID:     "panel_cf_record_id",
		SettingPanelCFRecordIDAAAA: "panel_cf_record_id_aaaa",
	} {
		if got != want {
			t.Errorf("clave %q, esperaba %q", got, want)
		}
		if slices.Contains(ProxySettingKeys, got) {
			t.Errorf("%q no puede estar en ProxySettingKeys", got)
		}
	}
}
