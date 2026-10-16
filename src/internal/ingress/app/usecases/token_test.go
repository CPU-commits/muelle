package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const tokenUnderTest = "cf-token-nuevo-1234567890abcdef"

func newTokenCheck(t *testing.T) (*cftest.Server, *CheckCloudflareToken) {
	t.Helper()
	cf := cftest.New(t)
	return cf, NewCheckCloudflareToken(cloudflare.New(cf.URL()))
}

// fieldError exige un *validate.Error en cloudflare_token, sin el token en
// el mensaje, y lo devuelve.
func fieldError(t *testing.T, zones *int, err error) *validate.Error {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok || ve.Field != "cloudflare_token" || zones != nil {
		t.Fatalf("esperaba un 422 en cloudflare_token: %v %v", zones, err)
	}
	if strings.Contains(ve.Error(), tokenUnderTest) {
		t.Fatalf("el mensaje trae el token: %q", ve.Error())
	}
	return ve
}

// Un token válido cuenta las zonas de todas las páginas.
func TestCheckTokenCountsZones(t *testing.T) {
	cf, uc := newTokenCheck(t)
	cf.AddToken(tokenUnderTest)
	for i := range 120 {
		cf.AddZone(fmt.Sprintf("z%03d", i), fmt.Sprintf("zona%03d.cl", i))
	}
	zones, err := uc.Execute(context.Background(), tokenUnderTest)
	if err != nil || zones == nil || *zones != 120 {
		t.Fatalf("Execute: %v %v", zones, err)
	}
}

// Un token que Cloudflare no conoce: ni verifica ni lista.
func TestCheckTokenRejected(t *testing.T) {
	cf, uc := newTokenCheck(t)
	cf.AddZone("z1", "tudominio.cl")
	zones, err := uc.Execute(context.Background(), tokenUnderTest)
	fieldError(t, zones, err)
}

// Un token de una cuenta no se verifica en /user/tokens/verify, pero lista
// zonas: se acepta (contrato §3.5).
func TestCheckTokenAccount(t *testing.T) {
	cf, uc := newTokenCheck(t)
	cf.AddAccountToken(tokenUnderTest)
	cf.AddZone("z1", "tudominio.cl")
	cf.AddZone("z2", "otro.cl")
	zones, err := uc.Execute(context.Background(), tokenUnderTest)
	if err != nil || zones == nil || *zones != 2 {
		t.Fatalf("Execute: %v %v", zones, err)
	}
}

// verify responde 200 con un token desactivado o vencido: es un rechazo, y
// el mensaje dice el estado (D31).
func TestCheckTokenInactive(t *testing.T) {
	for status, word := range map[string]string{"disabled": "desactivado", "expired": "vencido"} {
		t.Run(status, func(t *testing.T) {
			cf, uc := newTokenCheck(t)
			cf.SetTokenStatus(tokenUnderTest, status)
			cf.AddZone("z1", "tudominio.cl")
			zones, err := uc.Execute(context.Background(), tokenUnderTest)
			if ve := fieldError(t, zones, err); !strings.Contains(ve.Message, word) {
				t.Fatalf("el mensaje no dice %q: %q", word, ve.Message)
			}
		})
	}
}

// Un token válido que no ve ninguna zona, o al que /zones le niega el
// acceso, no sirve: el mensaje pide los permisos.
func TestCheckTokenWithoutZones(t *testing.T) {
	t.Run("sin zonas", func(t *testing.T) {
		cf, uc := newTokenCheck(t)
		cf.AddToken(tokenUnderTest)
		zones, err := uc.Execute(context.Background(), tokenUnderTest)
		if ve := fieldError(t, zones, err); !strings.Contains(ve.Message, "Zone:Read") || !strings.Contains(ve.Message, "DNS:Edit") {
			t.Fatalf("el mensaje no pide los permisos: %q", ve.Message)
		}
	})
	t.Run("403 en /zones", func(t *testing.T) {
		cf, uc := newTokenCheck(t)
		cf.AddToken(tokenUnderTest)
		cf.AddZone("z1", "tudominio.cl")
		cf.FailNext("GET", "/zones", 403, 9109, "Unauthorized to access requested resource")
		zones, err := uc.Execute(context.Background(), tokenUnderTest)
		if ve := fieldError(t, zones, err); !strings.Contains(ve.Message, "Zone:Read") {
			t.Fatalf("el mensaje no pide los permisos: %q", ve.Message)
		}
	})
}

// Si Cloudflare no responde (caído, un 5xx o un 429), es un 502.
func TestCheckTokenUnavailable(t *testing.T) {
	cases := map[string]func(cf *cftest.Server){
		"caído": func(cf *cftest.Server) { cf.SetDown(true) },
		"500":   func(cf *cftest.Server) { cf.FailNext("GET", "/user/tokens/verify", 500, 10000, "internal") },
		"429":   func(cf *cftest.Server) { cf.FailNext("GET", "/user/tokens/verify", 429, 10000, "rate limited") },
		"/zones": func(cf *cftest.Server) {
			cf.FailNext("GET", "/zones", 502, 10000, "bad gateway")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			cf, uc := newTokenCheck(t)
			cf.AddToken(tokenUnderTest)
			cf.AddZone("z1", "tudominio.cl")
			setup(cf)
			zones, err := uc.Execute(context.Background(), tokenUnderTest)
			if !errors.Is(err, shared.ErrCloudflareUnavailable) || zones != nil {
				t.Fatalf("Execute: %v %v", zones, err)
			}
			if _, ok := errors.AsType[*validate.Error](err); ok {
				t.Fatal("no es un 422")
			}
			if strings.Contains(err.Error(), tokenUnderTest) {
				t.Fatalf("el error trae el token: %v", err)
			}
		})
	}
}

// Un ctx cancelado por el cliente corta la verificación: el PATCH no
// guarda nada.
func TestCheckTokenCanceled(t *testing.T) {
	cf, uc := newTokenCheck(t)
	cf.AddToken(tokenUnderTest)
	cf.AddZone("z1", "tudominio.cl")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if zones, err := uc.Execute(ctx, tokenUnderTest); err == nil || zones != nil {
		t.Fatalf("Execute: %v %v", zones, err)
	}
}
