// Package natstest is a minimal in-process NATS server for tests: it speaks
// just enough of the text protocol (INFO, CONNECT, PING/PONG, PUB) to accept a
// client and keep what it publishes. It exists so the tests need no broker and
// no second dependency.
package natstest

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Message is one publication received by the server.
type Message struct {
	Subject string
	Data    []byte
}

// Server is a running test server.
type Server struct {
	ln net.Listener

	mu       sync.Mutex
	messages []Message
	connects []string
	conns    []net.Conn
	wg       sync.WaitGroup
}

// Accepted is how many connections clients have opened, CONNECT or not.
func (s *Server) Accepted() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Start listens on a free local port.
func Start() (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	s := &Server{ln: ln}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

// URL is the address clients connect to.
func (s *Server) URL() string { return "nats://" + s.ln.Addr().String() }

// Messages returns what has been published so far.
func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

// Connects returns the CONNECT lines clients sent, which is where a client's
// credentials travel.
func (s *Server) Connects() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.connects...)
}

// Close stops the server and drops its clients.
func (s *Server) Close() {
	_ = s.ln.Close()
	s.mu.Lock()
	for _, c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		s.wg.Add(1)
		go s.serve(conn)
	}
}

func (s *Server) serve(conn net.Conn) {
	defer s.wg.Done()
	defer func() { _ = conn.Close() }()

	info := `INFO {"server_id":"natstest","version":"2.10.0","host":"127.0.0.1","max_payload":1048576,"headers":true}` + "\r\n"
	if _, err := io.WriteString(conn, info); err != nil {
		return
	}
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "CONNECT"):
			s.mu.Lock()
			s.connects = append(s.connects, line)
			s.mu.Unlock()
		case line == "PING":
			if _, err := io.WriteString(conn, "PONG\r\n"); err != nil {
				return
			}
		case strings.HasPrefix(line, "PUB "):
			fields := strings.Fields(line)
			size, err := strconv.Atoi(fields[len(fields)-1])
			if err != nil || len(fields) < 3 {
				return
			}
			data := make([]byte, size+2) // payload and its CRLF
			if _, err := io.ReadFull(r, data); err != nil {
				return
			}
			s.mu.Lock()
			s.messages = append(s.messages, Message{Subject: fields[1], Data: data[:size]})
			s.mu.Unlock()
		}
	}
}
