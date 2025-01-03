// checkdeps generates a .deps file to help "make" update tarballs when
// source files change.
package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func macOSMetadata(name string) bool {
	idx := strings.LastIndexByte(name, '/')
	return idx > 0 && idx+2 < len(name) && name[idx+1] == '.' && name[idx+2] == '_'
}

// Generate a list of dependencies for `tr`.
func genTarDeps(deps, metaDeps []string, tarFile *os.File, srcdir,
	objdir string) ([]string, []string) {
	tarball := tarFile.Name()

	// Do we need to interpose a gzip reader?
	r := io.Reader(tarFile)
	if strings.HasSuffix(tarball, ".gz") {
		gzipReader, err := gzip.NewReader(tarFile)
		if err != nil {
			log.Fatalf("creating gzip reader for %s: %v", tarball, err)
		}
		defer gzipReader.Close()
		r = gzipReader
	}

	// Assume files in `tarball` are dependencies.
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("error reading header from %s: %v", tarball, err)
			}
			break
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		_, path, found := strings.Cut(hdr.Name, "/")
		if !found {
			log.Printf("no / in %s", hdr.Name)
			continue
		}

		// Prefer the file in `objdir` over that in `srcdir`.
		fixed := filepath.FromSlash(path)
		objpath := filepath.Join(objdir, fixed)
		srcpath := filepath.Join(srcdir, fixed)
		if _, err := os.Stat(objpath); err == nil {
			deps = append(deps, objpath)
		} else if _, err := os.Stat(srcpath); err == nil {
			deps = append(deps, srcpath)
		} else if macOSMetadata(hdr.Name) {
			// Mac OS tar inserts these into ircu2.tar by default.
			continue
		} else {
			// Complain about the unknown file.
			log.Printf("no source file found for %s", fixed)
		}

		// Makefiles are candidate meta-dependencies, but are listed
		// as Makefile.in (in the current packages).
		if strings.HasSuffix(hdr.Name, "/Makefile.in") {
			// Chop off ".in" when generating the converted path.
			// A generated Makefile should only be in `objdir`.
			fixed := filepath.FromSlash(path[:len(path)-3])
			objpath := filepath.Join(objdir, fixed)
			if _, err := os.Stat(objpath); err == nil {
				metaDeps = append(metaDeps, objpath)
			} else {
				log.Printf("no Makefile found for %s: %v", objpath, err)
			}
		}
	}

	return deps, metaDeps
}

// Generate a list of dependencies for `tarball`.
// If the tarball already exists, use its contents as a cue.
// Otherwise assume the contents of `srcdir` are used.
func genDeps(metaDeps []string, w *bufio.Writer, tarball, srcdir, objdir string) []string {
	deps := make([]string, 0, 32)

	// Try to read the tarball.
	tarFile, err := os.Open(tarball)
	if err == nil {
		deps, metaDeps = genTarDeps(deps, metaDeps, tarFile, srcdir, objdir)
	} else if !os.IsNotExist(err) {
		log.Printf("reading %s: %v", tarball, err)
	} else {
		// Assume every file in `srcdir` is a dependency.
		// We do not update metaDeps.
		filepath.WalkDir(srcdir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				deps = append(deps, path)
			}
			return nil
		})
	}

	// Sort the dependencies for easier reading.
	sort.Strings(deps)

	// Emit the dependency list.
	if len(deps) > 0 {
		w.WriteString(tarball + ":")
		for _, dep := range deps {
			w.WriteString(" \\\n\t" + dep)
		}
		w.WriteString("\n\n")
	}

	return metaDeps
}

func main() {
	outFile := ".deps"
	if len(os.Args) > 1 {
		outFile = os.Args[1]
	}
	f, err := os.CreateTemp(".", ".deps-tmp")
	if err != nil {
		log.Fatalf("creating temp file: %v", err)
	}
	bw := bufio.NewWriter(f)

	metaDeps := make([]string, 0, 32)
	metaDeps = genDeps(metaDeps, bw,
		"images/ircu2/iauthd-c/iauthd-c.tar.gz", "iauthd-c", "+iauthd-c")
	metaDeps = genDeps(metaDeps, bw,
		"images/ircu2/ircu2/ircu2.tar.gz", "ircu2", "+ircu2")
	metaDeps = genDeps(metaDeps, bw,
		"images/srvx-1.x/srvx-1.x.tar.gz", "srvx-1.x", "+srvx-1.x")

	// Write the dependency rule for .deps itself.
	sort.Strings(metaDeps)
	if len(metaDeps) > 0 {
		bw.WriteString(".deps:")
		for _, dep := range metaDeps {
			bw.WriteString(" \\\n\t" + dep)
		}
		bw.WriteString("\n\t$(GO) run ./tools/checkdeps.go $<")
	}

	if err = bw.Flush(); err != nil {
		log.Fatalf("flushing output: %v", err)
	}
	if err = f.Close(); err != nil {
		log.Printf("closing output file: %v", err)
	}
	if err = os.Rename(f.Name(), outFile); err != nil {
		log.Fatalf("renaming output file: %v", err)
	}
}
