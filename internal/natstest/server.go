// Package natstest is a minimal NATS server for tests: it accepts connections,
// answers the handshake and keeps every message published to it. It speaks just
// enough of the text protocol (INFO, CONNECT, PING/PONG, PUB) for a publisher,
// so the suites need no server binary and no extra dependency.
package natstest

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message is one publication received by the server.
type Message struct {
	Subject string
	Payload []byte
}

// Server is a running test server.
type Server struct {
	ln       net.Listener
	mu       sync.Mutex
	messages []Message
	conns    []net.Conn
	// refuse makes the server answer every PUB with -ERR and drop the
	// connection, the way a server does when permissions forbid the subject.
	refuse bool
}

// Start listens on a free local port.
func Start() (*Server, error) { return start(false) }

// StartRefusing is a server that accepts the handshake and then rejects any
// publication.
func StartRefusing() (*Server, error) { return start(true) }

func start(refuse bool) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	s := &Server{ln: ln, refuse: refuse}
	go s.accept()
	return s, nil
}

// URL is the address a client connects to.
func (s *Server) URL() string { return "nats://" + s.ln.Addr().String() }

// Close stops the server and drops its connections.
func (s *Server) Close() {
	_ = s.ln.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		_ = c.Close()
	}
}

// Connections is how many clients ever connected: zero proves none was tried.
func (s *Server) Connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Messages is a copy of what was published so far.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

// WaitFor blocks until n messages arrived or the timeout passed, and returns
// what it has.
func (s *Server) WaitFor(n int, timeout time.Duration) []Message {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if got := s.Messages(); len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return s.Messages()
}

func (s *Server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, c)
		s.mu.Unlock()
		go s.serve(c)
	}
}

func (s *Server) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	_, _ = io.WriteString(c, `INFO {"server_id":"natstest","version":"2.10.0","proto":1,"headers":true,"max_payload":1048576}`+"\r\n")
	r := bufio.NewReader(c)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "PING":
			_, _ = io.WriteString(c, "PONG\r\n")
		case strings.HasPrefix(line, "PUB "):
			fields := strings.Fields(line)
			size, err := strconv.Atoi(fields[len(fields)-1])
			if err != nil || size < 0 {
				return
			}
			payload := make([]byte, size+2)
			if _, err := io.ReadFull(r, payload); err != nil {
				return
			}
			if s.refuse {
				_, _ = io.WriteString(c, "-ERR 'Permissions Violation for Publish'\r\n")
				return
			}
			s.mu.Lock()
			s.messages = append(s.messages, Message{Subject: fields[1], Payload: payload[:size]})
			s.mu.Unlock()
		}
	}
}
