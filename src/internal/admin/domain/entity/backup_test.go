package entity

import (
	"testing"
	"time"
)

func TestBackupName(t *testing.T) {
	if got := BackupName(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)); got != "muelle-2026-10-10.db" {
		t.Fatalf("got %s", got)
	}
	// Cerca de la medianoche el día es el UTC, no el de la zona del reloj.
	tz := time.FixedZone("UTC-5", -5*3600)
	if got := BackupName(time.Date(2026, 10, 10, 20, 30, 0, 0, tz)); got != "muelle-2026-10-11.db" {
		t.Fatalf("got %s", got)
	}
	if got := BackupName(time.Date(2026, 10, 11, 1, 0, 0, 0, time.FixedZone("UTC+3", 3*3600))); got != "muelle-2026-10-10.db" {
		t.Fatalf("got %s", got)
	}
}

func TestParseBackupName(t *testing.T) {
	day, ok := ParseBackupName("muelle-2026-10-10.db")
	if !ok || !day.Equal(time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("day=%v ok=%v", day, ok)
	}
	for _, bad := range []string{
		"", "muelle-manual-antes-de-migrar.db", "muelle-2026-10-10.db.tmp", "muelle-2026-13-10.db",
		"muelle-2026-1-10.db", "otro-2026-10-10.db", "muelle-2026-10-10.sql", "muelle-2026-10-10.db.1.tmp",
	} {
		if _, ok := ParseBackupName(bad); ok {
			t.Errorf("%q no debería ser un nombre de copia diaria", bad)
		}
	}
}
