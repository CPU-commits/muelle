package usecases

import (
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// El contador de streams por principal: un token, una sesión y otro token
// cuentan aparte; release devuelve el lugar una sola vez y borra la entrada
// en 0.
func TestStreamSlots(t *testing.T) {
	s := NewStreamSlots(2)
	tok := entity.Principal{Token: &entity.TokenScope{ID: 7}}
	other := entity.Principal{Token: &entity.TokenScope{ID: 8}}
	session := entity.Principal{UserID: 1, SessionHash: "h"}

	r1, ok1 := s.Acquire(tok)
	r2, ok2 := s.Acquire(tok)
	if _, ok := s.Acquire(tok); !ok1 || !ok2 || ok {
		t.Fatalf("el tercero del mismo token: %v %v %v", ok1, ok2, ok)
	}
	if _, ok := s.Acquire(other); !ok {
		t.Fatal("otro token tiene sus lugares")
	}
	if _, ok := s.Acquire(session); !ok {
		t.Fatal("una sesión tiene sus lugares")
	}
	r1()
	r1() // una segunda llamada no devuelve otro lugar
	if _, ok := s.Acquire(tok); !ok {
		t.Fatal("el lugar devuelto se puede volver a tomar")
	}
	if _, ok := s.Acquire(tok); ok {
		t.Fatal("release devolvió dos lugares")
	}
	r2()
	s.mu.Lock()
	n := s.open["t:7"]
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("t:7 tiene %d abiertos, esperaba 1", n)
	}
}
