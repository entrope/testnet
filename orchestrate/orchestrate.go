// Main package for `orchestrate` binary.
// This constructs a Compose file that will run a specified script.
// It creates a folder with the basename of the input file to hold
// the various configuration files that are generated from the script.
package main

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"iter"
	"log"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/crypto/pbkdf2"
	"gopkg.in/yaml.v3"
)

var networkCIDR = flag.String("cidr", "10.11.12.0/24",
	"CIDR representation of test network IP address range")
var noGenerate = flag.Bool("q", false,
	"If set, do not create compose.yaml and related files")
var noExecute = flag.Bool("n", false,
	"If set, do not run the compose file")
var noCollect = flag.Bool("c", false,
	"If set, do not collect coverage data from the containers")
var toolName = flag.String("tool", "podman",
	"Compose tool to execute")
var lcovTool = flag.String("lcov", "lcov",
	"Coverage tool to execute")
var seedFlag = flag.String("seed", "",
	"Random seed to use (base64 encoded)")
var failed bool
var scriptName string
var seed []byte

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

// createBoss creates the "boss" service.
func createBoss() {
	// Skip the first address so that Podman can use it for DNS etc.
	nextAddr = nextAddr.Next()

	// Create the service.
	if err := makeService("boss", "boss"); err != nil {
		log.Fatalf("failed to create boss service: %v", err)
	}

	// Allow the boss to implement an ident server and know extra IPs.
	boss := compose.Services["boss"]
	// `podman build` is unstable, so we only use the Build directive
	// for images without profiling support :(
	boss.Build = "../../images/boss"
	boss.Configs = []ServiceConfig{{
		Source: "irc.script",
		Target: "/etc/irc.script",
	}}
	boss.Sysctls = map[string]string{
		"net.ipv4.ip_unprivileged_port_start": "113",
	}
}

// makeService adds a service named `name` with type `image` to the
// Compose application.
func makeService(name string, image string) error {
	if compose.Services["boss"] == nil && name != "boss" {
		createBoss()
	}

	svcNetwork := ServiceNetwork{}
	if nextAddr.Is4() {
		svcNetwork.IPv4Address = nextAddr.String()
	} else {
		svcNetwork.IPv6Address = nextAddr.String()
	}
	svc := &Service{
		Image: "localhost/coder-com/" + image,
		Networks: map[string]*ServiceNetwork{
			"inner": &svcNetwork,
		},
		PullPolicy: "never",
	}
	nextAddr = nextAddr.Next()
	compose.Services[name] = svc
	containers[name] = name
	return nil
}

// replaceSuffix replaces "..." at the end of `name` with `suffix`.
func replaceSuffix(name string) string {
	if strings.HasSuffix(name, "...") {
		return name[:len(name)-2] + suffix
	}

	return name
}

// cmdClient handles the CLIENT script command, to create a new client.
func cmdClient(words []string) error {
	if len(words) < 2 {
		return errors.New("Expected CLIENT <name>[@<client>] <server>[/tls] [<username>]")
	}

	// Parse out the command line arguments.
	name, runsOn := words[0], ""
	if idx := strings.IndexByte(name, '@'); idx > 0 {
		runsOn = name[idx+1:]
		name = name[:idx]
	}
	server := words[1]
	// Strip off /tls and :<port> from server name, if present.
	server, _ = strings.CutSuffix(server, "/tls")
	if idx := strings.LastIndexByte(server, ':'); idx > 0 {
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
		return errors.New("no existing server is named " + server)
	}

	// Clients run on the boss service.
	containers[name] = "boss"

	// If this client needs a dedicated IP address, assign one.
	if runsOn == "" {
		// Allocate an IP address.
		extraIP := nextAddr.String()
		nextAddr = nextAddr.Next()

		// Assign it to the container and ready it for /etc/hosts.
		bossSvc := compose.Services[containers[name]]
		bossSvc.ExtraHosts = append(bossSvc.ExtraHosts, name+":"+extraIP)
		nw := bossSvc.Networks["inner"]
		nw.LinkLocalIPs = append(nw.LinkLocalIPs, extraIP)
		bossSvc.Networks["inner"] = nw
	}
	return nil
}

// cmdServer creates a new IRC server.
func cmdServer(words []string) error {
	// Parse the command line arguments.
	if len(words) != 2 {
		return errors.New("expected SERVER <name> <image>")
	}
	name := replaceSuffix(words[0])
	image := words[1]

	if !strings.ContainsAny(name, ".") {
		return errors.New("server names must contain a dot")
	}
	if _, ok := containers[name]; ok {
		return errors.New("already have something named " + name)
	}

	return makeService(name, image)
}

// cmdSuffix adjusts the "standard" suffix for server or host names.
func cmdSuffix(words []string) error {
	if len(words) != 1 {
		return errors.New("expected SUFFIX <name>")
	}
	suffix = words[0]
	return nil
}

// scriptCommands maps a command token to the function that handles it.
var scriptCommands = map[string]func([]string) error{
	"CLIENT": cmdClient,
	"SERVER": cmdServer,
	"SUFFIX": cmdSuffix,
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
	Me       string
	IP       map[string]string
	ClientIP map[string]string
}

// writeConfig writes the config file for `name`.
func writeConfig(tmpl *template.Template, ips map[string]string) {
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
	fullPath := filepath.Join(host, file)
	dirPath := filepath.Dir(fullPath)
	if err := os.MkdirAll(dirPath, dirMode); err != nil {
		log.Println(err)
		return
	}

	// Build a subset of the IP mappings for clients.
	clientIP := make(map[string]string)
	for k, v := range ips {
		if !strings.ContainsRune(k, '.') {
			clientIP[k] = v
		}
	}

	// Create the file and execute the template.
	f, err := os.Create(fullPath)
	if err != nil {
		log.Println(err)
		return
	}
	if err = tmpl.Execute(f, &ConfigObject{
		Me:       host,
		IP:       ips,
		ClientIP: clientIP,
	}); err != nil {
		log.Fatal(err)
	}
	if err = f.Close(); err != nil {
		log.Println(err)
	}

	// Stash this as a configuration.
	cfgName := strings.ReplaceAll(tmpl.Name(), "/", "_")
	cfgName = strings.ReplaceAll(cfgName, ":", "-")
	compose.Configs[cfgName] = &ConfigOrSecret{
		File: host + file,
	}
	svc := compose.Services[host]
	svc.Configs = append(svc.Configs, ServiceConfig{
		Source: cfgName,
		Target: file,
	})
}

// makePassword generates a pseudorandom password using the orchestrator
// global key plus the salt (which is often an ASCII string).
// It returns a 16-character base64 string (with 96 bits of entropy).
func makePassword(salt string) string {
	pw := pbkdf2.Key(seed, []byte(salt), 4096, 12, sha256.New)
	return base64.RawURLEncoding.EncodeToString(pw)
}

func setup() {
	compose = Compose{
		Name:     scriptName,
		Services: make(map[string]*Service),
		Networks: make(map[string]*Network),
		Configs:  make(map[string]*ConfigOrSecret),
	}

	// Parse command-line flags.
	var err error
	if netMask, err = netip.ParsePrefix(*networkCIDR); err != nil {
		log.Fatalf("%s not parsed as a network prefix: %v", *networkCIDR, err)
	}
	netMask = netMask.Masked()
	nextAddr = netMask.Addr().Next() // first address is for the network

	// Parse the script file.
	tmpl = template.New("irc.tmpl")
	tmpl.Funcs(map[string]any{
		"password": makePassword,
	})
	tmpl, err = tmpl.ParseFiles("irc.tmpl")
	if err != nil {
		log.Fatalf("failed to parse irc.tmpl as a template file: %v", err)
	}
	sb := &strings.Builder{}
	if err = tmpl.Execute(sb, nil); err != nil {
		log.Fatalf("failed to evaluate %s: %v", scriptName, err)
	}
	scriptText := sb.String()
	if err = os.WriteFile("irc.script", []byte(scriptText), fileMode); err != nil {
		log.Fatalf("failed to write irc.script: %v", err)
	}
	sb = nil

	// Populate helper data structures.
	populateHelpers()
	compose.Networks["inner"] = &Network{
		Attachable: false,
		EnableIPv6: netMask.Addr().Is6(),
		Internal:   true,
		IPAM: IPAM{
			Driver: "default",
			Config: []IPAMConfig{{
				Subnet: netMask,
			}},
		},
	}

	// Create a config for the master script.
	compose.Configs["irc.script"] = &ConfigOrSecret{
		File: "irc.script",
	}

	// Split the script text into lines and process each.
	for lineno, line := range strings.Split(scriptText, "\n") {
		doScriptLine(line, lineno+1)
	}

	// Map service names to their IP address.
	ips := make(map[string]string)
	for k, v := range containers {
		if k == v {
			svcNetwork := compose.Services[k].Networks["inner"]
			addr := svcNetwork.IPv4Address
			if addr == "" {
				addr = svcNetwork.IPv6Address
			}
			ips[k] = addr
		}
	}

	// Write the config files.
	for _, t := range tmpl.Templates() {
		writeConfig(t, ips)
	}

	// Write out the Compose file.
	var composeText []byte
	if composeText, err = yaml.Marshal(&compose); err != nil {
		log.Fatalf("failed to format compose file: %v", err)
	}
	err = os.WriteFile("compose.yaml", composeText, fileMode)
	if err != nil {
		log.Fatalf("failed to write compose file: %v", err)
	}
}

// Reads lines from the output of a command, one at a time.
func execTool(args ...string) iter.Seq[string] {
	cmd := exec.Command(*toolName, args...)
	text, err := cmd.Output()
	if err != nil {
		log.Fatalf("%s %s failed: %v", *toolName, args[0], err)
	}
	buf := bytes.NewBuffer(text)
	return func(yield func(string) bool) {
		for {
			l, err := buf.ReadString('\n')
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				log.Fatalf("reading line '%s' failed: %v", l, err)
			}
			if !yield(l) {
				return
			}
		}
	}
}

func execToolMap(args ...string) iter.Seq2[string, string] {
	cmd := exec.Command(*toolName, args...)
	text, err := cmd.Output()
	if err != nil {
		log.Fatalf("%s %s failed: %v", *toolName, args[0], err)
	}
	buf := bytes.NewBuffer(text)
	return func(yield func(string, string) bool) {
		for {
			l, err := buf.ReadString('\n')
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				log.Fatalf("reading line '%s' failed: %v", l, err)
			}
			k, v, found := strings.Cut(l, " ")
			if !found {
				log.Fatalf("bad line from %s %s: %s", *toolName, args[0], l)
			}
			if !yield(k, v) {
				return
			}
		}
	}
}

type stringSet = map[string]struct{}

// If `hdr` is for a GCNO file, extract it from `tr`.
func extractGcnoFile(hdr *tar.Header, tr *tar.Reader) {
	// Ignore files that are not in the builder's home directory.
	const prefix = "home/coder-com/"
	if !strings.HasPrefix(hdr.Name, prefix) {
		return
	}

	// Ignore files that do not end with the right suffix.
	idx := strings.LastIndexByte(hdr.Name, '/')
	fileName := hdr.Name[idx:]
	pkg, found := strings.CutSuffix(fileName, "-gcno.tar.bz2")
	if !found {
		return
	}

	// Copy the file to the host directory.
	gcnoFile := filepath.Join("..", "..", "coverage", pkg, fileName)
	out, err := os.Create(gcnoFile)
	if err != nil {
		log.Fatalf("error creating %s: %v", gcnoFile, err)
	}
	if _, err := io.Copy(out, tr); err != nil {
		log.Fatalf("error copying %s: %v", gcnoFile, err)
	}
	if err = out.Close(); err != nil {
		log.Println(err)
	}
}

// Extracts GCNO files from the specified container's `build` stage.
func extractGcno(container string) {
	// What's the name of the image?
	cmd := exec.Command(*toolName, "inspect", container, "--format", "{{.ImageName}}")
	imageName, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("unable to retrieve ImageName for container %s: %v", container, err)
	}
	label, _, found := strings.Cut(string(imageName), ":")
	if !found {
		log.Fatalf("ImageName for %s had no colon: %s", container, imageName)
	}
	idx := strings.LastIndexByte(label, '/')
	name := label[idx+1:]

	// Try to create a container from the tagged build stage.
	// If that fails, build and tag it.
	tag := label + ":build"
	cmd = exec.Command(*toolName, "create", "--pull", "never", tag)
	containerID, err := cmd.CombinedOutput()
	if err != nil {
		contextPath := filepath.Join("..", "..", "images", name)
		buildCmd := exec.Command(*toolName, "build",
			"--target", "build",
			"-t", tag,
			contextPath)
		if _, err := buildCmd.CombinedOutput(); err != nil {
			log.Fatalf("failed to build %s: %v", tag, err)
		}

		cmd = exec.Command(*toolName, "create", "--pull", "never", tag)
		if containerID, err = cmd.CombinedOutput(); err != nil {
			log.Fatalf("failed to create image for %s: %v", tag, err)
		}
	}
	id := strings.TrimSpace(string(containerID))

	// Tidy up the container before we leave.
	defer func() {
		cmd := exec.Command(*toolName, "rm", id)
		if msg, err := cmd.CombinedOutput(); err != nil {
			log.Fatalf("failed to remove container %s: %v:\n%s\n", id, err, string(msg))
		}
	}()

	// Trawl the container's filesystem to find GCNO files.
	cmd = exec.Command(*toolName, "export", id)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("getting pipe for podman export %s: %v", id, err)
	}
	if err = cmd.Start(); err != nil {
		log.Fatalf("starting podman export %s: %v", id, err)
	}

	// Read the tarfile from the tool's stdout.
	tr := tar.NewReader(stdout)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			log.Fatalf("error reading from %s: %v", id, err)
		}

		extractGcnoFile(hdr, tr)
	}
}

// If `hdr` is a GCDA file, copies it to the profile working directory.
func collectHeader(hdr *tar.Header, tr *tar.Reader, done stringSet) stringSet {
	// Does it look like a file with coverage output?
	const prefix = "home/coder-com/irc/"
	if !strings.HasPrefix(hdr.Name, prefix) ||
		!strings.HasSuffix(hdr.Name, ".gcda") {
		return done
	}
	pkg, gcda, found := strings.Cut(hdr.Name[len(prefix):], "/")
	if !found {
		return done
	}
	pkgDir := filepath.Join("..", "..", "coverage", pkg)
	if strings.HasPrefix(gcda, "src/+build") {
		gcda = gcda[len("src/+build"):]
	}

	// Have we already processed a GCDA file for this package?
	gcdaDir := filepath.Join(pkgDir, "gcda")
	if _, ok := done[pkg]; !ok {
		done[pkg] = struct{}{}
		if err := os.RemoveAll(gcdaDir); err != nil && !os.IsNotExist(err) {
			log.Fatalf("RemoveAll %s: %v", gcdaDir, err)
		}
	}

	// Save the GCDA file.
	gcdaFile := filepath.Join(gcdaDir, filepath.FromSlash(gcda))
	fileDir := filepath.Dir(gcdaFile)
	if err := os.MkdirAll(fileDir, dirMode); err != nil && !os.IsExist(err) {
		log.Fatalf("MkdirAll %s: %v", fileDir, err)
	}

	// Copy the file to the host directory.
	out, err := os.Create(gcdaFile)
	if err != nil {
		log.Fatalf("error creating %s: %v", gcdaFile, err)
	}
	if _, err = io.Copy(out, tr); err != nil {
		log.Fatalf("error copying %s: %v", gcdaFile, err)
	}
	if err = out.Close(); err != nil {
		log.Println(err)
	}

	return done
}

// Collects output from the container with the specified ID.
func collectOutput(id string) {
	// `done` names the gcda directories we have created.
	done := stringSet{}

	// Run (the equivalent of) "podman export" on the container.
	cmd := exec.Command(*toolName, "export", id)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("getting pipe for podman export %s: %v", id, err)
	}
	if err = cmd.Start(); err != nil {
		log.Fatalf("starting podman export %s: %v", id, err)
	}

	// Read the tarfile that went to the tool's stdout.
	tr := tar.NewReader(stdout)
	for {
		// Read the next file header, if we can.
		hdr, err := tr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			log.Fatalf("error reading from %s: %v", id, err)
		}

		done = collectHeader(hdr, tr, done)
	}

	// For each GCDA directory we processed, save its data and remove
	// the working GCDA directory.
	for pkg := range done {
		// Do we need to extract GCNO files for this image?
		gcnoDir := filepath.Join("..", "..", "coverage", pkg, "gcno")
		if _, err := os.Stat(gcnoDir); errors.Is(err, os.ErrNotExist) {
			extractGcno(id)
		}

		// Call the script to generate a coverage report.
		cmd := exec.Command("sh", "-e", "coverage.sh")
		cmd.Dir = filepath.Join("..", "..", "coverage", pkg)
		if txt, err := cmd.CombinedOutput(); err != nil {
			fmt.Print(string(txt))
			log.Fatalf("running coverage.sh for %s in %s (in %s): %v", pkg, id, cmd.Dir, err)
		}
		gcdaDir := filepath.Join(cmd.Dir, "gcda")
		if err := os.RemoveAll(gcdaDir); err != nil {
			log.Fatalf("removing %s: %v", gcdaDir, err)
		}
	}
}

// execute runs the test script.
func execute() {
	cmd := exec.Command(*toolName, "compose", "up")
	if _, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("%s compose up failed: %v", *toolName, err)
	}
}

// collect collects profile output from all containers for our test script.
func collect() {
	prefix := filepath.Base(flag.Arg(0)) + "-"
	for id, name := range execToolMap("ps", "-a", "--format", "{{.ID}} {{.Names}}") {
		if strings.HasPrefix(name, prefix) {
			collectOutput(id)
		}
	}
}

//revive:disable:cyclomatic
func main() {
	// Parse the command line.
	collectFiles := make([]string, 0, 4)
	flag.Func("collect", "containers from which to collect data files", func(id string) error {
		collectFiles = append(collectFiles, id)
		return nil
	})
	flag.Parse()
	var err error
	if *seedFlag != "" {
		if seed, err = base64.RawURLEncoding.DecodeString(*seedFlag); err != nil {
			log.Fatalf("parsing random seed: %v", err)
		}
	}
	if seed == nil || len(seed) < 1 {
		seed = make([]byte, 32)
		if _, err = rand.Read(seed); err != nil {
			log.Fatalf("creating seed: %v", err)
		}
	}

	// Switch to the script directory.
	scriptDir := flag.Arg(0)
	if scriptDir == "" {
		log.Fatalf("Usage: %s <script-dir>", os.Args[0])
	}
	if err := os.Chdir(scriptDir); err != nil {
		log.Fatalf("Chdir %s: %v", scriptDir, err)
	}
	scriptName = filepath.Base(scriptDir)

	// Should we collect profiling outputs?
	if len(collectFiles) > 0 {
		for _, id := range collectFiles {
			collectOutput(id)
		}
		return
	}

	// Generate and execute the test scripts.
	if !*noGenerate && !failed {
		log.Print("creating Compose application")
		setup()
	}
	if !*noExecute && !failed {
		log.Print("launching Compose application")
		execute()
	}
	if !*noCollect && !failed {
		log.Print("collecting coverage data")
		collect()
	}
}
