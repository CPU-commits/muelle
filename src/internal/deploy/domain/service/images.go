package service

import (
	"errors"

	"github.com/distribution/reference"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
)

// ChooseImages elige la ref pedida de cada servicio de la configuración
// (contrato §2.2): el override; si no, la pedida del deploy actual si la ref
// de la configuración no cambió desde ese deploy; si no, la de la
// configuración. Los overrides de servicios que no están en config se ignoran
// (el caso de uso los avisa en el log). current puede ser nil. No modifica
// sus argumentos.
//
// Nunca se le quita el digest a una ref: así un digest fijado a propósito (un
// override, la configuración o un rollback) se mantiene en los deploys sin
// cuerpo que siguen, y un rollback no se deshace solo.
func ChooseImages(config, overrides map[string]string, current *entity.Deploy) map[string]string {
	out := make(map[string]string, len(config))
	for svc, cfgRef := range config {
		if ref, ok := overrides[svc]; ok {
			out[svc] = ref
			continue
		}
		if current != nil {
			prevCfg, okCfg := current.ConfigImages[svc]
			prevReq, okReq := current.RequestedImages[svc]
			if okCfg && okReq && prevCfg == cfgRef {
				out[svc] = prevReq
				continue
			}
		}
		out[svc] = cfgRef
	}
	return out
}

// ResolvedRef es la ref resuelta: la pedida con el tag explícito y el digest
// (`nginx` → `nginx:latest@sha256:…`), en la forma familiar de Docker. Una
// pedida con digest queda igual (no gana un `:latest`), y su digest tiene que
// ser d: el registry devolvió el de otro manifiesto, o la ref cambió.
func ResolvedRef(requested, d string) (string, error) {
	named, err := reference.ParseNormalizedNamed(requested)
	if err != nil {
		return "", errors.New("la ref no es válida")
	}
	if pinned, ok := named.(reference.Digested); ok {
		if pinned.Digest().String() != d {
			return "", errors.New("el registry devolvió otro digest que el que fija la ref")
		}
		return reference.FamiliarString(named), nil
	}
	// reference valida el digest al parsear (digest.Parse: algoritmo y largo).
	full, err := reference.ParseNormalizedNamed(reference.TagNameOnly(named).String() + "@" + d)
	if err != nil {
		return "", errors.New("el digest no es válido")
	}
	if _, ok := full.(reference.Digested); !ok {
		return "", errors.New("el digest no es válido")
	}
	return reference.FamiliarString(full), nil
}

// ImageHost es el host de la credencial de una imagen (contrato §2.10):
// reference.Domain(ParseNormalizedNamed(ref)), docker.io para Docker Hub.
func ImageHost(ref string) (string, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", errors.New("la ref no es válida")
	}
	return reference.Domain(named), nil
}

// PinnedDigest es el digest de una ref que ya lo trae, o "". No es un atajo
// para no consultar el registry (decisión 1 del plan, contrato §2.2): el paso 2
// consulta todas las refs y lo usa solo para comprobar que Images.Digest
// devolvió ese mismo digest.
func PinnedDigest(ref string) string {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ""
	}
	if d, ok := named.(reference.Digested); ok {
		return d.Digest().String()
	}
	return ""
}
