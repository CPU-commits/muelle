package entity

import "time"

const (
	backupPrefix = "muelle-"
	backupSuffix = ".db"
	backupLayout = "2006-01-02"
)

// BackupName es el nombre de la copia diaria de t, en UTC: muelle-AAAA-MM-DD.db.
func BackupName(t time.Time) string {
	return backupPrefix + t.UTC().Format(backupLayout) + backupSuffix
}

// ParseBackupName dice si name es exactamente el nombre de una copia diaria
// y devuelve su día. Cualquier otro archivo (copias manuales, temporales) no lo es.
func ParseBackupName(name string) (time.Time, bool) {
	if len(name) != len(backupPrefix)+len(backupLayout)+len(backupSuffix) ||
		name[:len(backupPrefix)] != backupPrefix || name[len(name)-len(backupSuffix):] != backupSuffix {
		return time.Time{}, false
	}
	day, err := time.Parse(backupLayout, name[len(backupPrefix):len(name)-len(backupSuffix)])
	if err != nil {
		return time.Time{}, false
	}
	return day, true
}
