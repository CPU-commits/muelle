package docker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// frame arma un frame del stream multiplexado de Docker (stdcopy): 1 byte de
// stream, 3 de relleno y el largo en big endian.
func frame(stream byte, payload string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

const details = "com.docker.swarm.node.id=n1,com.docker.swarm.service.id=s1,com.docker.swarm.task.id=t1"

func readAll(t *testing.T, s LogStream) []LogLine {
	t.Helper()
	var out []LogLine
	for {
		l, err := s.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
}

func TestLogStreamMultiplexed(t *testing.T) {
	var b bytes.Buffer
	b.Write(frame(1, "2026-10-12T00:00:01.123456789Z "+details+" hola mundo\n"))
	b.Write(frame(2, "2026-10-12T00:00:02Z "+details+" error: algo  con  espacios\n"))
	// Una línea partida en dos frames, y dos líneas en un frame.
	b.Write(frame(1, "2026-10-12T00:00:03Z "+details+" mitad"))
	b.Write(frame(1, " y mitad\n2026-10-12T00:00:04Z "+details+" otra\n"))
	// Sin detalles (Docker deja el campo vacío) y con \r\n.
	b.Write(frame(1, "2026-10-12T00:00:05Z  sin detalles\r\n"))

	s := newLogStream(io.NopCloser(&b), false)
	got := readAll(t, s)
	if len(got) != 5 {
		t.Fatalf("%d líneas: %+v", len(got), got)
	}
	first := got[0]
	if !first.Time.Equal(time.Date(2026, 10, 12, 0, 0, 1, 123456789, time.UTC)) || first.Stream != StreamStdout ||
		first.TaskID != "t1" || first.NodeID != "n1" || first.ServiceID != "s1" || first.Text != "hola mundo" {
		t.Fatalf("primera: %+v", first)
	}
	if got[1].Stream != StreamStderr || got[1].Text != "error: algo  con  espacios" {
		t.Fatalf("segunda: %+v", got[1])
	}
	if got[2].Text != "mitad y mitad" || got[3].Text != "otra" {
		t.Fatalf("partidas: %+v / %+v", got[2], got[3])
	}
	if got[4].Text != "sin detalles" || got[4].TaskID != "" {
		t.Fatalf("sin detalles: %+v", got[4])
	}
}

// Un servicio con tty: Docker manda el texto sin multiplexar.
func TestLogStreamTTY(t *testing.T) {
	raw := "2026-10-12T00:00:01Z " + details + " uno\n2026-10-12T00:00:02Z " + details + " dos"
	got := readAll(t, newLogStream(io.NopCloser(strings.NewReader(raw)), true))
	if len(got) != 2 || got[0].Text != "uno" || got[1].Text != "dos" || got[1].Stream != StreamStdout {
		t.Fatalf("tty: %+v", got)
	}
}

// Los valores de los detalles van con escape de URL (url.QueryEscape).
func TestParseLogLineEscapedDetails(t *testing.T) {
	l := parseLogLine(StreamStdout, "2026-10-12T00:00:01Z com.docker.swarm.task.id=t%2B1,otro=a%20b texto")
	if l.TaskID != "t+1" || l.Text != "texto" {
		t.Fatalf("%+v", l)
	}
	// Una línea sin timestamp (no debería pasar) se entrega entera.
	if l := parseLogLine(StreamStdout, "suelta"); l.Text != "suelta" || !l.Time.IsZero() {
		t.Fatalf("sin timestamp: %+v", l)
	}
}

// Un frame que dice medir más de lo razonable corta el stream con un error
// en vez de reservar memoria sin límite.
func TestLogStreamRejectsHugeFrame(t *testing.T) {
	h := make([]byte, 8)
	h[0] = 1
	binary.BigEndian.PutUint32(h[4:], 1<<31)
	s := newLogStream(io.NopCloser(bytes.NewReader(h)), false)
	if _, err := s.Next(); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("frame enorme: %v", err)
	}
}

// Una línea muy larga se corta en maxLogLine bytes: no puede llenar la memoria.
func TestLogStreamLongLine(t *testing.T) {
	long := strings.Repeat("x", maxLogLine*2)
	raw := "2026-10-12T00:00:01Z " + details + " " + long + "\n2026-10-12T00:00:02Z " + details + " corta\n"
	got := readAll(t, newLogStream(io.NopCloser(strings.NewReader(raw)), true))
	if len(got) < 2 || got[len(got)-1].Text != "corta" {
		t.Fatalf("después de una línea larga sigue la siguiente: %d líneas", len(got))
	}
	for _, l := range got {
		if len(l.Text) > maxLogLine {
			t.Fatalf("una línea de %d bytes pasa el máximo de %d", len(l.Text), maxLogLine)
		}
	}
}

func TestLogStreamClose(t *testing.T) {
	r, w := io.Pipe()
	s := newLogStream(r, true)
	done := make(chan error, 1)
	go func() {
		_, err := s.Next()
		done <- err
	}()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Next después de Close debe devolver un error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close no desbloqueó Next")
	}
	w.Close()
}
