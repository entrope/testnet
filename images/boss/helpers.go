package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// Suffix is the hostname suffix for this Compose application.
var Suffix string

// ReplaceSuffix replaces "..." at the end of `name` with `suffix`.
func ReplaceSuffix(name string) string {
	if strings.HasSuffix(name, "...") {
		return name[:len(name)-2] + Suffix
	}

	return name
}

// IsClosedConnError returns true if err is an error that is typically
// returned when using a closed network connection.
func IsClosedConnError(err error) bool {
	return err != nil && (errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF))
}

// IrcTrim trims leading spaces and tabs, and trailing CRLF, from `line`.
func IrcTrim(line string) string {
	line = strings.TrimLeft(line, " \t")
	line = strings.TrimRight(line, "\r\n")
	return line
}

// appendSplit splits `line` in an IRC-like fashion and appends its
// tokens to `parts`.
//
// The first token is interpreted as a command and appended as-is.
// Following tokens are delimited by spaces, but a token starting with
// ':' means the rest of the line is a single (line-final) token.
func appendSplit(parts []string, line string) []string {

	for ii, ll, first := 0, len(line), true; ii < ll; first = false {
		// Skip leading whitespace.
		for ; ii < ll && line[ii] == ' '; ii++ {
		}

		// Is the rest of the line a single token?
		if !first && line[ii] == ':' {
			return append(parts, line[ii+1:])
		}

		// Scan to end of token.
		jj := ii
		for ; ii < ll && line[ii] != ' '; ii++ {
		}

		// Append the token.
		parts = append(parts, line[jj:ii])
	}

	return parts
}

// ScriptSplitLine works like IrcSplitLine except that:
//   - Lines starting with '#' return nil, as comments.
//   - Lines starting with ':<name> <text>' are translated to
//     "SEND", "<name>", "<text>".
func ScriptSplitLine(line string) []string {
	// Ignore blank lines and comments.
	if line = IrcTrim(line); len(line) == 0 || line[0] == '#' {
		return nil
	}

	// Is this a SEND-type line?
	if line[0] == ':' {
		name, rest, found := strings.Cut(line, " ")
		if !found {
			panic("Invalid script syntax " + line)
		}
		return []string{"SEND", name[1:], rest}
	}

	return appendSplit(make([]string, 0, 4), line)
}

// IrcSplitLine splits `line` in an IRC-client-like fashion.
//
// The returned slice is nil if the line is blank.
// Otherwise, the slice contains the source and command names, followed
// by any arguments in the line.
//
// If the line starts with ':', then the rest of the first token is the
// source name and the command name is the second token; otherwise, the
// source name is "" and the command name is the first token.
func IrcSplitLine(line string) []string {
	// Ignore blank lines.
	if line = IrcTrim(line); len(line) == 0 {
		return nil
	}

	// Allocate a slice.
	parts := make([]string, 1, 7)

	// Does this have a prefix?
	if line[0] == ':' {
		ii, ll := 1, len(line)
		for ; ii < ll && line[ii] != ' '; ii++ {
		}
		if ii == ll {
			fmt.Printf("Bogus IRC line %v\n", line)
			return nil
		}
		parts[0] = line[1:ii]
		line = line[ii+1:]
	} // else parts[0] is already ""

	return appendSplit(parts, line)
}

// SplitAddress splits the host address and port out of a network address.
func SplitAddress(addr string) (host string, port uint16) {
	// Find the port-number suffix.
	idx := strings.LastIndexByte(addr, ':')
	if idx < 0 {
		return "", 0
	}

	// Parse the host part of addr.
	if addr[0] == '[' && idx > 1 && addr[idx-1] == ']' {
		host = addr[1 : idx-1]
	} else {
		host = addr[0:idx]
	}

	// Parse port number part of addr.
	bigPort, err := strconv.ParseUint(addr[idx+1:], 10, 16)
	if err != nil || bigPort > 65535 {
		return "", 0
	}
	port = uint16(bigPort)

	return host, port
}
