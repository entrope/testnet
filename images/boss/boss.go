// The testnet driver executes irc.script.
// Each client entity gets one receiving goroutine.
// The main goroutine sends data and makes control flow decisions.
package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var clients = make(map[string]*ClientConn, 64)
var ident Ident
var waitClients []*ClientConn

func clientUnknown(name string) {
	fmt.Printf("ERROR BADNAME %s :Unknown client\n", name)
}

// doSendText sends a line from a client to its server.
// Syntax: `SEND [!]<name> :<text>` or `:[!]<name> <text>`
// The optional '!' prefix suppresses the usual rate limiting logic.
// Returns true if the send should be retried later.
func doSendText(name string, text string) bool {
	// Do we know the client?
	client, ok := clients[name]
	if !ok {
		clientUnknown(name)
		return false
	}

	// Is this client waiting on text?
	if len(client.Expect) > 0 {
		waitClients = append(waitClients, client)
		return true
	}

	// Should we use the default rate-limiting?
	rateLimit := true
	if name[0] == '!' {
		rateLimit = false
		name = name[1:]
	}

	// Expand the text to send, apply rate limiting, then send..
	text = client.Expand(text)
	if rateLimit {
		client.RateLimit(text)
	}
	client.Send(text)
	return false
}

// addExpect adds an expected line for the specified client.
// Syntax: `EXPECT <name>[@<timeout>][!] :<regexp>`
// The timeout defaults to 10 seconds.
// The optional `!` after a timeout specifies that a timeout is fatal
// for the entire script.
// Named patterns in the regexp are captured in the client's varliables.
func addExpect(name, pattern string) {
	exp := Expectation{Deadline: time.Now()}

	// Is it fatal?
	if name[0] == '!' {
		exp.Fatal = true
		name = name[1:]
	}

	// Is there a timeout?
	timeout := "10s"
	if idx := strings.LastIndexByte(name, '@'); idx > 0 {
		timeout = name[idx+1:]
		name = name[:idx]
	}
	sec, err := strconv.ParseFloat(timeout, 64)
	if err != nil {
		fmt.Printf("ERROR COMMAND EXPECT :invalid duration %s\n", timeout)
		return
	}
	exp.Deadline.Add(time.Duration(math.Round(1e9 * sec)))

	// Look up the client so we can expand "pattern".
	if client, ok := clients[name]; ok {
		var err error
		pattern = client.Expand(pattern)
		exp.Pattern, err = regexp.Compile(pattern)
		if err != nil {
			fmt.Printf("ERROR COMMAND EXPECT :invalid pattern: %v\n", err)
			return
		}
		client.Expect = append(client.Expect, exp)
	} else {
		clientUnknown(name)
	}
}

// doWait records that we want to wait for the named clients.
// Syntax: `WAIT [<name ...>]`
// Returns true if the WAIT should be retried later.
func doWait(names []string) bool {
	if len(names) == 0 {
		// Default to waiting for all clients with expectations.
		for _, client := range clients {
			if len(client.Expect) > 0 {
				waitClients = append(waitClients, client)
			}
		}
	} else {
		// Wait for the named clients.
		for _, name := range names {
			if client, ok := clients[name]; ok {
				waitClients = append(waitClients, client)
			} else {
				clientUnknown(name)
			}
		}
	}

	return len(waitClients) > 0
}

// checkWaitClients processes expectations for some set of clients.
// It removes clients from `waitClients` if they are satisfied.
// It returns true if `waitClients` is empty.
func checkWaitClients() bool {
	jj := 0
	for ii := 0; ii < len(waitClients); ii++ {
		if len(waitClients[ii].Expect) > 0 {
			waitClients[jj] = waitClients[ii]
			jj++
		}
	}
	for ii := jj; ii < len(waitClients); ii++ {
		waitClients[ii] = nil
	}
	waitClients = waitClients[:jj]
	return jj == 0
}

// createClient connects a new client to an IRC server.
// Syntax: `CLIENT <name>[@<other>] server[:port][/tls] [username]`
func createClient(argv []string, textChan chan<- TextLine) {
	// Parse argv[].
	name, server, username := argv[1], argv[2], ""
	if argc := len(argv); argc > 3 {
		username = argv[3]
	}
	fmt.Printf("CLIENT %s %s %s\n", name, server, username)

	client := NewClient(name, server, username, textChan)
	clients[client.Nickname] = client
}

// executeLine executes one line of script.
// It returns false on success, and true if the line should be retried.
func executeLine(text string, textChan chan<- TextLine) bool {
	parts := ScriptSplitLine(text)
	if parts == nil {
		return false
	}

	switch parts[0] {
	case "CIDR":
		// do nothing; this is handled by the orchestrator
	case "CLIENT":
		createClient(parts, textChan)
	case "EXPECT":
		addExpect(parts[1], parts[2])
	case "SERVER":
		// do nothing; this is handled by the orchestrator
	case "SEND":
		return doSendText(parts[1], parts[2])
	case "SUFFIX":
		Suffix = parts[1]
	case "WAIT":
		return doWait(parts[1:])
	default:
		fmt.Printf("ERROR COMMAND %s :%s\n", parts[0], text)
	}

	return false
}

var retryLine string

// doWork processes I/O and returns true if the script should continue.
func doWork(signalChannel <-chan os.Signal, textChan chan TextLine, s *bufio.Scanner) bool {
	select {
	case sig := <-signalChannel:
		if sig == syscall.SIGTERM || sig == syscall.SIGINT {
			fmt.Printf("got signal %v\n", sig)
			return false
		}
		fmt.Printf("ERROR SIGNAL %d :Unexpected %s\n", sig, sig.String())

	case text := <-textChan:
		text.Handle()
		return true

	default:
		// Are we waiting for any clients?
		if len(waitClients) > 0 && !checkWaitClients() {
			time.Sleep(300 * time.Millisecond)
			return true
		}

		// Do we need to read a new line from the file?
		if retryLine == "" {
			if !s.Scan() {
				if err := s.Err(); err != nil {
					log.Printf("error scanning input: %v", err)
				} else {
					log.Printf("end of script (%v)", err)
				}
				return false
			}
			retryLine = s.Text()
			log.Printf("%s\n", retryLine)
		}

		// Execute it.
		if !executeLine(retryLine, textChan) {
			retryLine = ""
		}

		return true
	}

	log.Fatal("ERROR SELECT :fell off end of doWork")
	return false
}

func main() {
	// Catch the signals we care about.
	signalChannel := make(chan os.Signal, 4)
	signal.Notify(signalChannel, syscall.SIGINT)
	signal.Notify(signalChannel, syscall.SIGTERM)

	// Open our input script.
	scriptName := "/etc/irc.script"
	if len(os.Args) > 1 {
		scriptName = os.Args[1]
	}
	input, err := os.Open(scriptName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to open %s: %v\n", scriptName, err)
		os.Exit(1)
	}

	// Start our ident server.
	if err := ident.Listen(); err != nil {
		fmt.Printf("failed to listen for ident: %v\n", err)
	}

	// Run the main event loop.
	go ident.Serve()
	textChan := make(chan TextLine, 64)

	// Create a scanner, which will panic if the input line is too long.
	s := bufio.NewScanner(input)
	s.Buffer(make([]byte, 32768), 512)
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("ERROR INPUT :%v\n", r)
		}
	}()

	// Work until we cannot.
	for doWork(signalChannel, textChan, s) {
	}

	// Close everything.
	fmt.Printf("shutting down")
	for _, c := range clients {
		_ = c.Close()
	}
	_ = ident.Close()
	_ = input.Close()
}
