package docker

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// maxLogLine es el máximo de una línea de log; una más larga se corta en
	// pedazos de este tamaño, así una app no puede llenar la memoria de muelle.
	maxLogLine = 16 << 10
	// maxFrame es el máximo de un frame del stream multiplexado.
	maxFrame = 1 << 20
)

// Los detalles que Docker agrega a cada línea del log de un servicio
// (ServiceLogs con Details, url.QueryEscape en clave y valor).
const (
	attrNodeID    = "com.docker.swarm.node.id"
	attrServiceID = "com.docker.swarm.service.id"
	attrTaskID    = "com.docker.swarm.task.id"
)

// logStream lee el log que devuelve Docker con Timestamps y Details: cada
// línea es "<RFC3339Nano> <k=v,k=v> <texto>". Sin TTY viene multiplexado en
// frames (stdcopy); con TTY, como texto plano.
type logStream struct {
	rc  io.ReadCloser
	br  *bufio.Reader
	tty bool
	buf []byte // para leer con TTY

	mu      sync.Mutex // Close puede llamarse desde otra goroutine mientras Next espera
	closed  bool
	pending []LogLine
	partial map[string][]byte // lo que queda sin "\n" de cada stream
	err     error
}

func newLogStream(rc io.ReadCloser, tty bool) *logStream {
	return &logStream{rc: rc, br: bufio.NewReaderSize(rc, 32<<10), tty: tty, partial: map[string][]byte{}}
}

var errStreamClosed = errors.New("docker: el stream de logs está cerrado")

func (s *logStream) Next() (LogLine, error) {
	for len(s.pending) == 0 {
		if s.err != nil {
			return LogLine{}, s.err
		}
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return LogLine{}, errStreamClosed
		}
		if err := s.fill(); err != nil {
			s.flush()
			if errors.Is(err, io.EOF) {
				s.err = io.EOF
			} else if s.isClosed() {
				s.err = errStreamClosed
			} else {
				s.err = fmt.Errorf("docker: leyendo el log: %w", err)
			}
		}
	}
	l := s.pending[0]
	s.pending = s.pending[1:]
	return l, nil
}

func (s *logStream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *logStream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.rc.Close()
}

// fill lee un frame (o un pedazo, con TTY) y deja sus líneas completas en pending.
func (s *logStream) fill() error {
	if s.tty {
		if s.buf == nil {
			s.buf = make([]byte, 32<<10)
		}
		n, err := s.br.Read(s.buf)
		s.feed(StreamStdout, s.buf[:n])
		return err
	}
	var h [8]byte
	if _, err := io.ReadFull(s.br, h[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return io.EOF // un header a medias al final: se termina igual
		}
		return err
	}
	size := binary.BigEndian.Uint32(h[4:])
	if size > maxFrame {
		return fmt.Errorf("un frame de %d bytes supera el máximo", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(s.br, payload); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return io.EOF
		}
		return err
	}
	switch h[0] {
	case 1:
		s.feed(StreamStdout, payload)
	case 2:
		s.feed(StreamStderr, payload)
	case 3: // systemerr: un error del daemon a mitad del stream
		return fmt.Errorf("el daemon cortó el log: %s", strings.TrimSpace(string(payload)))
	}
	return nil
}

// feed suma bytes de un stream y emite las líneas que se completan.
func (s *logStream) feed(stream string, data []byte) {
	buf := s.partial[stream]
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			buf = append(buf, data...)
			data = nil
		} else {
			buf = append(buf, data[:i]...)
			data = data[i+1:]
			s.emit(stream, buf)
			buf = buf[:0]
		}
		for len(buf) >= maxLogLine {
			s.emit(stream, buf[:maxLogLine])
			buf = append(buf[:0], buf[maxLogLine:]...)
		}
	}
	s.partial[stream] = buf
}

func (s *logStream) emit(stream string, raw []byte) {
	s.pending = append(s.pending, parseLogLine(stream, strings.TrimSuffix(string(raw), "\r")))
}

// flush emite lo que quedó sin "\n" al terminar el stream.
func (s *logStream) flush() {
	for _, stream := range []string{StreamStdout, StreamStderr} {
		if buf := s.partial[stream]; len(buf) > 0 {
			s.emit(stream, buf)
			s.partial[stream] = nil
		}
	}
}

// parseLogLine separa "<timestamp> <detalles> <texto>". Una línea sin
// timestamp (un pedazo de una línea demasiado larga) va entera como texto.
func parseLogLine(stream, raw string) LogLine {
	l := LogLine{Stream: stream, Text: raw}
	ts, rest, ok := strings.Cut(raw, " ")
	t, err := time.Parse(time.RFC3339Nano, ts)
	if !ok || err != nil {
		return l
	}
	l.Time = t.UTC()
	details, text, _ := strings.Cut(rest, " ")
	if details != "" && !strings.Contains(details, "=") {
		l.Text = rest // sin detalles: todo es texto
		return l
	}
	l.Text = text
	for kv := range strings.SplitSeq(details, ",") {
		k, v, _ := strings.Cut(kv, "=")
		if uk, err := url.QueryUnescape(k); err == nil {
			k = uk
		}
		if uv, err := url.QueryUnescape(v); err == nil {
			v = uv
		}
		switch k {
		case attrNodeID:
			l.NodeID = v
		case attrServiceID:
			l.ServiceID = v
		case attrTaskID:
			l.TaskID = v
		}
	}
	return l
}
