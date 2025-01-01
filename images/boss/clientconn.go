package main

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ClientConn represents a connection to an IRC server.
type ClientConn struct {
	// Name is the unique name assigned for this client.
	Name string

	// Nickname is the client's current nickname.
	Nickname string

	// LastJoined is the last channel the client joined.
	LastJoined string

	// Server is the name of the server this client connected to.
	Server string

	// Expect is a list of regular expressions we expect this client to see.
	Expect []Expectation

	// conn is the underlying network connection.
	conn net.Conn

	// since is the virtual time at which the IRC server will accept the
	// last data we sent.
	// This is like cli_since() / con_since() in ircu2.
	since time.Time

	// scan is the scanner that reads lines from the server.
	scanner *bufio.Scanner

	// registered is set to true once the client is fully registered.
	registered bool

	// registeredCond is a condition variable around `registered`.
	// `registeredCond.L` is also used to serialize sending data.
	registeredCond *sync.Cond

	// vars is a map of captured variables for this client.
	vars map[string]string
}

// Expectation records one expected line from a server.
type Expectation struct {
	// Pattern describes what we want to match.
	Pattern *regexp.Regexp

	// Deadline is the time at which we give up on the expectation.
	Deadline time.Time

	// Fatal is true if a failed expectation should stop the script.
	Fatal bool
}

// TextLine represents one line of received text.
type TextLine struct {
	// Source identifies the connection that received the line.
	Source *ClientConn

	// Text is the received line, with up to one each carriage return
	// and newline character trimmed from the end.
	Text string

	// Err represents an error that occurred while reading data.
	// If an error occurs, it will be the last TextLine from the
	// goroutine.
	Err error
}

// Handle processes an incoming line of text for a client.
func (tl *TextLine) Handle() {
	// Was there a read error?
	if tl.Err != nil {
		fmt.Printf("ERROR CLIENT %s :%v\n", tl.Source.Name, tl.Err)
		return
	}

	// If there was no source prefix, add one.
	if tl.Text[0] != ':' && tl.Source.Server != "" {
		tl.Text = fmt.Sprintf(":%s %s", tl.Source.Server, tl.Text)
	}

	// Does it match an expectation?
	if len(tl.Source.Expect) > 0 {
		if m := tl.Source.Expect[0].Pattern.FindStringSubmatch(tl.Text); m != nil {
			// Save any named subexpressions.
			for idx, name := range tl.Source.Expect[0].Pattern.SubexpNames() {
				if name != "" {
					tl.Source.vars[name] = m[idx]
				}
			}

			// Drop this expectation.
			n := copy(tl.Source.Expect, tl.Source.Expect[1:])
			tl.Source.Expect = tl.Source.Expect[:n]
		}
	}

	// Handle commands like PING.
	f := strings.Fields(tl.Text)
	if f[1] == "PING" {
		tl.Source.Send("PONG :" + f[len(f)-1])
	}
}

// NewClient creates a new client with the specified (decorated) name,
// server and username.
// `name` should be <name>[@<other>].
// `server` should be <server>[:<port>][/tls].
// `username` may be empty to not give the client an ident response.
func NewClient(name, server, username string, textChan chan<- TextLine) *ClientConn {
	// Split nickname from the rest of `name`.
	nickname, host, hosted := strings.Cut(name, "@")
	if !hosted {
		host = nickname
	}

	// Create and run the client.
	client := &ClientConn{
		Name:           nickname,
		Nickname:       nickname,
		Server:         server, // may be modified by client.Run()
		registeredCond: sync.NewCond(&sync.Mutex{}),
	}

	// Launch it.  This will also register the ident response, if needed.
	go client.Run(host, username, textChan)

	// Return the new client.
	return client
}

// Close closes the connection.
func (c *ClientConn) Close() error {
	return c.conn.Close()
}

// CopyLines reads lines from c, delivering them to textChan.
// It will typically be run as a goroutine.
func (c *ClientConn) CopyLines(textChan chan<- TextLine) {
	msg := TextLine{Source: c}

	// c.scanner.Scan() will panic on an overly long line.
	defer func() {
		if r := recover(); r != nil {
			// c.scanner.Scan() only panics with a string.
			msg.Err = errors.New(r.(string))
			textChan <- msg
		} else {
			msg.Err = c.scanner.Err()
			if msg.Err == nil {
				msg.Err = io.EOF
			}
			textChan <- msg
		}
	}()

	for c.scanner.Scan() {
		msg.Text = c.scanner.Text()
		fmt.Printf("%s <- %s\n", c.Name, msg.Text)
		textChan <- msg
	}
}

// Expand will expand any named variables in `text`.
func (c *ClientConn) Expand(text string) string {
	return os.Expand(text, func(name string) string {
		switch name {
		case "me":
			return c.Nickname
		case "channel":
			return c.LastJoined
		default:
			v, ok := c.vars[name]
			if !ok {
				panic("unknown client variable " + name)
			}
			return v
		}
	})
}

// RateLimit applies rate limiting for a client command.
func (c *ClientConn) RateLimit(text string) {
	// Calculate how far ahead we can burst.
	now := time.Now()
	limit := now.Add(time.Duration(9) * time.Second)

	// Compare c.since to the limits.
	if c.since.After(limit) {
		// Sleep until we get close enough to c.Since.
		time.Sleep(c.since.Sub(limit))
	} else if now.After(c.since) {
		// Update c.Since to now.
		c.since = now
	} // else keep c.Since (in the near future)

	// Add the cost of this message to c.Since.
	cost := time.Second * time.Duration(2+len(text)/120)
	c.since = c.since.Add(cost)
}

// Send expands `text` and sends the client with optional rate-limiting.
func (c *ClientConn) Send(text string) {
	fmt.Printf("%s -> %s\n", c.Name, text)

	// Interpret selected commands like NICK and JOIN.
	f := strings.Fields(text)
	switch f[0] {
	case "JOIN":
		idx := strings.LastIndexByte(f[1], ',')
		c.LastJoined = f[1][idx+1:] // idx == -1 works here
	case "NICK":
		c.Nickname = f[1]
	}

	// Make sure we are registered before sending.
	c.registeredCond.L.Lock()
	defer c.registeredCond.L.Unlock()
	for !c.registered {
		c.registeredCond.Wait()
	}

	// Send it.
	if _, err := io.WriteString(c.conn, text+"\r\n"); err != nil {
		fmt.Printf("ERROR SOCKET %s :%v\n", c.Name, err)
	}
}

// NTuple describes the client's connection.
func (c *ClientConn) NTuple() (res NTuple) {
	res.LocalAddr, res.LocalPort = SplitAddress(c.conn.LocalAddr().String())
	res.RemoteAddr, res.RemotePort = SplitAddress(c.conn.RemoteAddr().String())
	return res
}

// finishRegistration finishes the client's registration.
// This waits until the server sends an 001 (WELCOME) message, and
// handles any PING before that.
// Returns true on success, false if something went wrong.
func (c *ClientConn) finishRegistration() {
	// c.scanner.Scan() will panic with a string on an overly long line.
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("ERROR SOCKET %s :%s\n", c.Name, r)
			c.registered = false
		}
	}()

scanLoop:
	for c.scanner.Scan() {
		// Was there an error reading the line?
		if err := c.scanner.Err(); err != nil {
			fmt.Printf("ERROR SOCKET %s :%v\n", c.Name, err)
			return
		}

		// See if the line is a type that we handle specially.
		text := c.scanner.Text()
		fmt.Printf("%s <- %s\n", c.Name, text)
		f := IrcSplitLine(text)
		lf := len(f)
		switch f[1] {
		case "001":
			break scanLoop
		case "PING":
			pong := fmt.Sprintf("PONG :%s\r\n", f[lf-1])
			_, _ = io.WriteString(c.conn, pong)
		}
	}

	// Record that we are registered.
	c.registered = true
	c.registeredCond.Broadcast()
}

// Run connects to the server and reads data from it.
// It is intended to run as a goroutine.
func (c *ClientConn) Run(host, username string, textChan chan<- TextLine) {
	// What server behaviors should we use?
	server, useTLS := strings.CutSuffix(c.Server, "/tls")
	server, portStr, _ := strings.Cut(server, ":")
	if portStr == "" {
		portStr = "6667"
	}
	server = ReplaceSuffix(server)

	// Look up host names.
	localAddr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		panic("failed to resolve host IP: " + err.Error())
	}

	// Initiate the TCP connection.
	dialer := &net.Dialer{LocalAddr: localAddr}
	c.Server = net.JoinHostPort(host, portStr)
	tcp, err := dialer.Dial("tcp", c.Server)
	if err != nil {
		panic("failed to connect to server: " + err.Error())
	}

	// Should we report a username for this client?
	if username == "" {
		username = c.Nickname
	} else {
		ident.Conns.Store(c.NTuple(), username)
	}

	// Should we run TLS on top of this connection?
	if useTLS {
		cfg := &tls.Config{
			ServerName:         server,
			InsecureSkipVerify: true,
		}
		c.conn = tls.Client(tcp, cfg)
	} else {
		c.conn = tcp
	}

	// Create scanner to read from the connection.
	c.scanner = bufio.NewScanner(c.conn)
	c.scanner.Buffer(make([]byte, 2048), 512)

	// Register client with IRC.
	// The 0 is the initial mode, the _ is unused / reserved.
	hello := fmt.Sprintf("USER %s 0 _ :%s\r\nNICK %s\r\n",
		username, c.Nickname, c.Nickname)
	_, err = io.WriteString(c.conn, hello)
	if err != nil {
		fmt.Printf("failed to register: %v\n", err)
	}

	// Try to finish registration.
	c.finishRegistration()
	if !c.registered {
		return
	}

	// Run CopyLines() for the rest of this goroutine.
	c.CopyLines(textChan)
}
