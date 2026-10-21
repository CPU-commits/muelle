package service

import "strings"

// MatchZone devuelve la zona de hostname: la del sufijo más largo, por
// etiquetas completas (api.a.example.com cae en a.example.com antes que en
// example.com, y badexample.com no cae en example.com). El ápice cae en su
// propia zona. Los nombres de zona se comparan en minúsculas; hostname ya
// viene en su forma canónica (validate.Hostname).
func MatchZone(hostname string, zones []Zone) (Zone, bool) {
	var best Zone
	found := false
	for _, z := range zones {
		name := strings.ToLower(z.Name)
		if hostname != name && !strings.HasSuffix(hostname, "."+name) {
			continue
		}
		if !found || len(name) > len(best.Name) {
			best, found = z, true
		}
	}
	return best, found
}
