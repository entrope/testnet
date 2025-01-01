// The testnet driver executes irc.script.
// Each client entity gets one receiving goroutine.
// The main goroutine sends data and makes control flow decisions.
package main

import (
	"bufio"
	"fmt"
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
func doSendText(name string, text string) {
	// Should we use the default rate-limiting?
	rateLimit := true
	if name[0] == '!' {
		rateLimit = false
		name = name[1:]
	}

	// Do we know the client?
	if client, ok := clients[name]; ok {
		text = client.Expand(text)
		if rateLimit {
			client.RateLimit(text)
		}
		client.Send(text)
	} else {
		clientUnknown(name)
	}
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
	if name[len(name)-1] == '!' {
		exp.Fatal = true
		name = name[:len(name)-1]
	}

	// Is there a timeout?
	if idx := strings.LastIndexByte(name, '@'); idx > 0 {
		sec, err := strconv.ParseFloat(name[idx+1:], 64)
		if err != nil {
			fmt.Printf("ERROR COMMAND EXPECT :invalid duration %s\n", name[idx+1:])
			return
		}
		exp.Deadline.Add(time.Duration(math.Round(1e9 * sec)))
		name = name[:idx]
	} else {
		exp.Deadline.Add(time.Duration(10) * time.Second)
	}

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
// Syntax: `WAIT <name ...>`
func doWait(names []string) {
	for _, name := range names {
		if client, ok := clients[name]; ok {
			waitClients = append(waitClients, client)
		} else {
			clientUnknown(name)
		}
	}
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
	if argc := len(argv); argc > 2 {
		username = argv[argc-1]
	}

	client := NewClient(name, server, username, textChan)
	clients[client.Nickname] = client
}

func executeLine(text string, textChan chan<- TextLine) {
	fmt.Println(text)
	parts := ScriptSplitLine(text)
	if parts == nil {
		return
	}

	switch parts[0] {
	case "CLIENT":
		createClient(parts, textChan)
	case "EXPECT":
		addExpect(parts[1], parts[2])
	case "SERVER":
		// do nothing; these are handled by the test driver
	case "SEND":
		doSendText(parts[1], parts[2])
	case "SUFFIX":
		Suffix = parts[1]
	case "WAIT":
		doWait(parts[1:])
	default:
		fmt.Printf("ERROR COMMAND %s :%s\n", parts[0], text)
	}
}

// doWork processes I/O and returns true if the script should continue.
func doWork(signalChannel <-chan os.Signal, textChan chan TextLine, f *os.File) (res bool) {
	res = true
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 32768), 512)
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("ERROR INPUT :%v\n", r)
			res = false
		}
	}()

	select {
	case sig := <-signalChannel:
		if sig == syscall.SIGTERM || sig == syscall.SIGINT {
			fmt.Printf("got signal %v\n", sig)
			return false
		}
		fmt.Printf("ERROR SIGNAL %d :Unexpected %s\n", sig, sig.String())

	case text := <-textChan:
		text.Handle()

	default:
		// Are we waiting for any clients?
		if len(waitClients) > 0 && !checkWaitClients() {
			time.Sleep(300 * time.Millisecond)
			return true
		}

		// Read and execute a line of input, if we can.
		if s.Scan() {
			executeLine(s.Text(), textChan)
			return true
		}

		// Otherwise we encountered EOF or an error.
		if err := s.Err(); err != nil {
			fmt.Printf("ERROR INPUT :%v\n", err)
		}
		fmt.Printf("script eof (?)\n")
		return false
	}

	return res
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
	for doWork(signalChannel, textChan, input) {
	}

	// Close everything.
	fmt.Printf("shutting down")
	for _, c := range clients {
		_ = c.Close()
	}
	_ = ident.Close()
	_ = input.Close()
}
