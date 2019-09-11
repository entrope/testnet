// Main package for `orchestrate` binary.
// This constructs a Compose file that will run a specified script.
// It creates a folder with the basename of the input file to hold
// the various configuration files that are generated from the script.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

var networkCIDR = flag.String("cidr", "10.0.0.0/8",
	"CIDR representation of test network IP address range")
var bossImage = flag.String("boss", "localhost/testnet:boss",
	"Image name for the 'boss' (test manager) container")
var ircdImage = flag.String("ircd", "localhost/testnet:ircd",
	"Image name for ircd containers")
var entityImage = flag.String("entity", "localhost/testnet:entity",
	"Image name for entity containers")
var svcImage = flag.String("srvx", "testnet:srvx",
	"Image name for service containers")
var toolName = flag.String("tool", "podman",
	"Compose tool to execute")

const dirMode = os.FileMode(0750)
const fileMode = os.FileMode(0640)

// netMask is the parsed form of `networkCIDR`.
var netMask netip.Prefix

// nextAddr is the next address to assign from `netMask`.
var nextAddr netip.Addr

// compose is the Compose file being constructed.
var compose Compose

// tmpl is the top-level template for the Compose file.
var tmpl *template.Template

// Suffix to use for IRC server names.
var suffix string

// containers maps IRC server names, service names, or client names to
// the ID of the container that hosts that server, service, or client.
// Entries are identities except for clients that share a container with
// a previously defined client.
var containers = make(map[string]string)

// isSpaceAry indicates whether char `x` is IRC whitespace.
var isSpaceAry [256]bool

// ircIsSpace returns true iff `ch` is IRC whitespace.
func ircIsSpace(ch byte) bool {
	return isSpaceAry[ch]
}

func makeService(name string, image string) error {
	svc := &Service{
		Image:      image,
		Networks:   []string{"inner"},
		PullPolicy: "never",
		Sysctls: map[string]string{
			// allow identd
			"net.ipv4.ip_unprivileged_port_start": "113",
		},
	}
	if nextAddr.Is4() {
		svc.IPv4Address = nextAddr
	} else {
		svc.IPv6Address = nextAddr
	}
	nextAddr = nextAddr.Next()
	compose.Services[name] = svc
	containers[name] = name
	return nil
}

func replaceSuffix(name string) string {
	if strings.HasSuffix(name, "...") {
		return name[:len(name)-2] + suffix
	}

	return name
}

// cmdClient handles the CLIENT script command, to create a new client.
func cmdClient(words []string) error {
	if len(words) < 2 {
		return errors.New("Expected CLIENT <name>[@<client>] <server> [<username>]")
	}

	// Parse out the command line arguments.
	name, runsOn := words[0], ""
	if idx := strings.IndexByte(name, '@'); idx > 0 {
		runsOn = name[idx+1:]
		name = name[:idx]
	}
	server := words[1]
	// Strip off :<port> from server name, if present.
	if idx := strings.IndexByte(server, ':'); idx > 0 {
		server = server[:idx]
	}
	server = replaceSuffix(server)

	// Sanity-check formats and consistency.
	if strings.ContainsAny(name, ".") {
		return errors.New("client names cannot contain a dot")
	}
	if _, ok := containers[name]; ok {
		return errors.New("already have something named " + name)
	}
	if runsOn != "" {
		if strings.ContainsAny(runsOn, ".") {
			return errors.New("cannot run a client on a server")
		}
		if _, ok := containers[runsOn]; !ok {
			return errors.New("unknown peer client " + runsOn)
		}
	}
	if strings.IndexByte(server, '.') < 0 {
		return errors.New("server name must contain a dot")
	}
	if _, ok := containers[server]; !ok {
		return errors.New("No existing server is named " + server)
	}

	if runsOn != "" {
		containers[name] = runsOn
		return nil
	}

	return makeService(name, *entityImage)
}

func cmdIrcd(words []string) error {
	// Parse the command line arguments.
	if len(words) != 1 {
		return errors.New("Expected IRCD <name>")
	}
	name := replaceSuffix(words[0])

	if !strings.ContainsAny(name, ".") {
		return errors.New("Server names must contain a dot")
	}
	if _, ok := containers[name]; ok {
		return errors.New("Already have something named " + name)
	}

	return makeService(name, *ircdImage)
}

func cmdHub(words []string) error {
	// Parse the command line arguments.
	if len(words) != 1 {
		return errors.New("expected HUB <name>")
	}
	name := replaceSuffix(words[0])

	if !strings.ContainsAny(name, ".") {
		return errors.New("server names must contain a dot")
	}
	if _, ok := containers[name]; ok {
		return errors.New("already have something named " + name)
	}

	return makeService(name, *ircdImage)
}

func cmdService(words []string) error {
	// Parse the command line arguments.
	if len(words) != 1 {
		return errors.New("expected SERVICE <name>")
	}
	name := replaceSuffix(words[0])

	if !strings.ContainsAny(name, ".") {
		return errors.New("service names must contain a dot")
	}
	if _, ok := containers[name]; ok {
		return errors.New("already have something named " + name)
	}

	return makeService(name, *svcImage)
}

func cmdSuffix(words []string) error {
	if len(words) != 1 {
		return errors.New("expected SUFFIX <name>")
	}
	suffix = words[0]
	return nil
}

// scriptCommands maps a command token to the function that handles it.
var scriptCommands = map[string]func([]string) error{
	"CLIENT":  cmdClient,
	"HUB":     cmdHub,
	"IRCD":    cmdIrcd,
	"SERVICE": cmdService,
	"SUFFIX":  cmdSuffix,
}

// doScriptLine executes the command in line.  If an error occurs, it
// reports it with lineno.
func doScriptLine(line string, lineno int) {
	// Trim leading and trailing whitespace.
	line = strings.Trim(line, "\r\n\t ")

	// Ignore lines that are blank or start with #.
	if len(line) == 0 || line[0] == '#' {
		return
	}

	// Split the line like an IRC command.
	parts := make([]string, 0, 7)
	for ii := 0; ii < len(line); ii++ {
		// Scan past whitespace (or to end of string).
		for ; ii < len(line) && ircIsSpace(line[ii]); ii++ {
		}

		// Scan to next whitespace (or end of string).
		jj := ii + 1
		for ; jj < len(line) && !ircIsSpace(line[jj]); jj++ {
		}

		// Does the word start with a colon?
		if line[ii] != ':' {
			parts = append(parts, line[ii:jj])
			ii = jj
		} else if ii == 0 {
			parts = append(parts, "SEND", line[1:jj])
		} else {
			parts = append(parts, line[ii+1:])
			break
		}
	}

	// Dispatch the command, ignoring unrecognized commands.
	if cmd, ok := scriptCommands[parts[0]]; ok {
		if err := cmd(parts[1:]); err != nil {
			fmt.Printf("ERROR line %d (%s): %v\n", lineno, parts[0], err)
		}
	}
}

// populateHelpers initializes runtime arrays such as `isSpaceAry`.
func populateHelpers() {
	isSpaceAry[' '] = true
	// horizontal tab, newline, vertical tab, form feed, carriage return
	for ii := 9; ii < 14; ii++ {
		isSpaceAry[ii] = true
	}
}

// ConfigObject is passed to config-file templates.
type ConfigObject struct {
	Me string
	IP map[string]string
}

// writeConfig writes the config file for `name`.
func writeConfig(scriptDir string, tmpl *template.Template, ips map[string]string) {
	// Does this look like <host>:<file> for a known container host?
	host, file, found := strings.Cut(tmpl.Name(), ":")
	if !found {
		return
	}
	host = replaceSuffix(host)
	if _, ok := containers[host]; !ok {
		fmt.Printf("Unknown host for template %s:%s\n", host, file)
		return
	}

	// Can we create the directory the file exists in?
	fullPath := path.Join(scriptDir, host, file)
	dirPath := path.Dir(fullPath)
	if err := os.MkdirAll(dirPath, dirMode); err != nil {
		fmt.Printf("Unable to mkdir %s: %v\n", dirPath, err)
		return
	}

	// Create the file and execute the template.
	f, err := os.Create(fullPath)
	if err != nil {
		fmt.Printf("Error creating %s: %v\n", fullPath, err)
		return
	}
	tmpl.Execute(f, &ConfigObject{
		Me: host,
		IP: ips,
	})
	if err = f.Close(); err != nil {
		fmt.Printf("Error closing %s: %v\n", fullPath, err)
	}

	// Stash this as a configuration.
	compose.Configs[tmpl.Name()] = &ConfigOrSecret{
		File: host + file,
	}
	svc := compose.Services[host]
	svc.Configs = append(svc.Configs, ServiceConfig{
		Source: tmpl.Name(),
		Target: file,
	})
}

func setup() {
	// Parse command line.
	flag.Parse()
	scriptDir := flag.Arg(0)
	compose = Compose{
		Name:     path.Base(scriptDir),
		Services: make(map[string]*Service),
		Networks: make(map[string]*Network),
		Configs:  make(map[string]*ConfigOrSecret),
	}
	if compose.Name == "" {
		fmt.Printf("Usage: %s <script-dir>\n", scriptDir)
		os.Exit(1)
	}

	// Parse command-line flags.
	var err error
	if netMask, err = netip.ParsePrefix(*networkCIDR); err != nil {
		fmt.Printf("%s not parsed as a network prefix: %v\n", *networkCIDR, err)
		os.Exit(1)
	}
	netMask = netMask.Masked()
	nextAddr = netMask.Addr().Next() // first address is for the network

	// Parse the script file.
	scriptFile := path.Join(scriptDir, "irc.tmpl")
	tmpl, err = template.ParseFiles(scriptFile)
	if err != nil {
		fmt.Printf("Failed to parse %s as a template file: %v\n", scriptFile, err)
		os.Exit(1)
	}
	sb := &strings.Builder{}
	if err = tmpl.Execute(sb, nil); err != nil {
		fmt.Printf("Failed to evaluate %s: %v\n", scriptFile, err)
		os.Exit(1)
	}
	scriptText := sb.String()
	scriptPath := path.Join(scriptDir, "irc.script")
	if err = os.WriteFile(scriptPath, []byte(scriptText), fileMode); err != nil {
		fmt.Printf("Failed to write %s: %v\n", scriptPath, err)
		os.Exit(1)
	}
	sb = nil

	// Populate helper data structures.
	populateHelpers()
	compose.Networks["inner"] = &Network{
		Attachable: false,
		EnableIPv6: netMask.Addr().Is6(),
		Internal:   true,
		/* IPAM: IPAM{
			Config: []IPAMConfig{{
				Subnet: netMask,
			}},
		},*/
	}

	// Create the 'boss' service.
	if err := makeService("boss", *bossImage); err != nil {
		fmt.Printf("Failed to create boss service: %v\n", err)
		os.Exit(1)
	}
	compose.Configs["irc.script"] = &ConfigOrSecret{
		File: "irc.script",
	}
	compose.Services["boss"].Configs = []ServiceConfig{{
		Source: "irc.script",
		Target: "/home/coder-com/etc/irc.script",
	},
	}

	// Split the script text into lines and process each.
	for lineno, line := range strings.Split(scriptText, "\n") {
		doScriptLine(line, lineno+1)
	}

	// Map service names to their IP address.
	ips := make(map[string]string)
	for k, v := range containers {
		if k == v {
			addr := compose.Services[k].IPv4Address
			if !addr.IsValid() {
				addr = compose.Services[k].IPv6Address
			}
			ips[k] = addr.String()
		}
	}

	// Write the config files.
	for _, t := range tmpl.Templates() {
		writeConfig(scriptDir, t, ips)
	}

	// Write out the Compose file.
	var composeText []byte
	if composeText, err = yaml.Marshal(&compose); err != nil {
		fmt.Printf("Failed to format compose file: %v\n", err)
		os.Exit(1)
	}
	yamlPath := path.Join(scriptDir, "compose.yaml")
	err = os.WriteFile(yamlPath, composeText, fileMode)
	if err != nil {
		fmt.Printf("Failed to write compose file: %v\n", err)
		os.Exit(1)
	}
}

func execute() {
	cmd := exec.Command(*toolName, "compose", "up")
	if cmd == nil {
		return
	}
	cmd.Dir = flag.Arg(0)
	if _, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("%s compose up failed: %v\n", *toolName, err)
		os.Exit(1)
	}
}

func collect() {
	// TODO: collect coverage files or other reports from each container
}

func main() {
	setup()
	execute()
	collect()
}
