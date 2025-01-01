package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// NTuple identifies a network connection's endpoints.
// It is a 5-tuple without the protocol ID, which is assumed to be TCP.
type NTuple struct {
	// LocalAddr is a text representation of the local address, without
	// port or square brackets.
	LocalAddr string

	// RemoteAddr is a text representation of the remote address, without
	// port or square brackets.
	RemoteAddr string

	// LocalPort is the local port number.
	LocalPort uint16

	// RemotePort is the remote port number.
	RemotePort uint16
}

// Ident implements an RFC 1413 ident server.
type Ident struct {
	// Listener is the listener for the server.
	Listener net.Listener

	// Timeout is how long each connection will wait for data.
	Timeout time.Duration

	// Conns maps NTuple to string responses.
	Conns sync.Map
}

// serveOne performs a single ident lookup.
func (svc *Ident) serveOne(conn net.Conn) {
	// The longest normal query will be "12345, 12345\r\n".
	var rbuf [24]byte
	defer conn.Close()

	// Read the port pair from the socket.
	conn.SetDeadline(time.Now().Add(svc.Timeout))
	n, err := conn.Read(rbuf[:])
	if err != nil {
		return
	}

	// Construct the lookup key.
	var tuple NTuple
	s := strings.TrimRight(string(rbuf[0:n]), "\r\n")
	count, err := fmt.Sscanf(s, "%d, %d", &tuple.LocalPort, &tuple.RemotePort)
	if count != 2 || err != nil {
		return
	}
	tuple.LocalAddr = conn.LocalAddr().String()
	tuple.RemoteAddr = conn.RemoteAddr().String()

	// Construct our response.
	var resp string
	if v, ok := svc.Conns.Load(tuple); ok {
		resp = "ERROR : NO-USER"
	} else {
		resp = "GO : " + v.(string)
	}

	// Send our response.
	text := fmt.Sprintf("%s : %s\r\n", s, resp)
	conn.Write([]byte(text))
}

// Listen makes the ident server start listening on its server port.
func (svc *Ident) Listen() error {
	var err error

	// Should we default a timeout?
	if svc.Timeout == 0 {
		svc.Timeout = time.Second
	}

	// Do we need to create a listener?
	if svc.Listener == nil {
		svc.Listener, err = net.Listen("tcp", ":113")
	}

	return err
}

// Serve runs the ident server.
// If `svc.Listener` is nil, creates one with default parameters.
// This terminates when `svc.Listener` is closed.
func (svc *Ident) Serve() {
	// Process connections forever.
	for {
		// Accept a connection.
		conn, err := svc.Listener.Accept()
		if err != nil {
			// net.ErrClosed indicates the listener was closed.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}

		go svc.serveOne(conn)
	}
}

// Close shuts down the server.
func (svc *Ident) Close() error {
	return svc.Listener.Close()
}
