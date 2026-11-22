// Package arduinocli finds arduino-cli, and installs it when it is missing:
// the official release for this system downloaded from Arduino, unpacked
// where Forma looks for it, then the Uno's AVR core and the Servo library
// installed through it. It has no window, so it is tested headlessly
// (against a local server and a stand-in for the tool).
package arduinocli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BaseURL is where Arduino publishes arduino-cli's latest release.
var BaseURL = "https://downloads.arduino.cc/arduino-cli"

// Exe is the tool's file name here.
func Exe() string {
	if runtime.GOOS == "windows" {
		return "arduino-cli.exe"
	}
	return "arduino-cli"
}

// Dir is where Forma installs it (and looks for it): on Windows where
// Arduino's own installer puts it, elsewhere in the user's config folder.
func Dir() string {
	if la := os.Getenv("LOCALAPPDATA"); runtime.GOOS == "windows" && la != "" {
		return filepath.Join(la, "arduino-cli", "bin")
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "forma", "arduino-cli", "bin")
	}
	return filepath.Join(os.TempDir(), "forma-arduino-cli")
}

// Find is arduino-cli's path: $FORMA_ARDUINO_CLI, the PATH, or Dir; "" if
// it isn't installed.
func Find() string {
	if p := os.Getenv("FORMA_ARDUINO_CLI"); p != "" {
		if fileExists(p) {
			return p
		}
		return "" // set, and not there: as if not installed
	}
	if p, err := exec.LookPath("arduino-cli"); err == nil {
		return p
	}
	if p := filepath.Join(Dir(), Exe()); fileExists(p) {
		return p
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Archive is the release archive's name for an OS and architecture.
func Archive(goos, goarch string) (string, error) {
	os := map[string]string{"windows": "Windows", "linux": "Linux", "darwin": "macOS"}[goos]
	arch := map[string]string{"amd64": "64bit", "arm64": "ARM64", "386": "32bit"}[goarch]
	if os == "" || arch == "" {
		return "", fmt.Errorf("no arduino-cli release for %s/%s", goos, goarch)
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "arduino-cli_latest_" + os + "_" + arch + ext, nil
}

// Progress is told what is happening: a stage and how far through it
// (0..1, or -1 when it can't tell).
type Progress func(stage string, frac float64)

// Installer installs arduino-cli; the zero value installs the real one.
type Installer struct {
	BaseURL string // default BaseURL
	Dir     string // default Dir()
	GOOS    string // default runtime.GOOS
	GOARCH  string // default runtime.GOARCH
	// Run runs the tool (default: the real command, its output returned)
	Run func(ctx context.Context, exe string, args ...string) ([]byte, error)
	// Client fetches the archive (default http.DefaultClient)
	Client *http.Client
}

// Steps are what Setup runs once the tool is there: the board index, the
// Uno's core (the AVR compiler and libraries), the Servo library.
var Steps = [][]string{
	{"core", "update-index"},
	{"core", "install", "arduino:avr"},
	{"lib", "install", "Servo"},
}

// Setup downloads arduino-cli if it isn't installed, then installs the AVR
// core and the Servo library through it (doing nothing already done). It
// returns the tool's path.
func (in Installer) Setup(ctx context.Context, progress Progress) (string, error) {
	if progress == nil {
		progress = func(string, float64) {}
	}
	exe := ""
	if in.Dir == "" && in.BaseURL == "" {
		exe = Find()
	}
	if exe == "" {
		var err error
		if exe, err = in.Download(ctx, progress); err != nil {
			return "", err
		}
	}
	run := in.Run
	if run == nil {
		run = runTool
	}
	for i, step := range Steps {
		progress("arduino-cli "+strings.Join(step, " "), float64(i)/float64(len(Steps)))
		if out, err := run(ctx, exe, step...); err != nil {
			msg := strings.TrimSpace(string(out))
			if i := strings.LastIndex(msg, "\n"); i >= 0 {
				msg = msg[i+1:]
			}
			return exe, fmt.Errorf("arduino-cli %s: %v %s", strings.Join(step, " "), err, msg)
		}
	}
	progress("done", 1)
	return exe, nil
}

// Download fetches the release archive and unpacks the tool into Dir,
// returning its path.
func (in Installer) Download(ctx context.Context, progress Progress) (string, error) {
	goos, goarch := in.GOOS, in.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	name, err := Archive(goos, goarch)
	if err != nil {
		return "", err
	}
	base := in.BaseURL
	if base == "" {
		base = BaseURL
	}
	client := in.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/"+name, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("downloading arduino-cli: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading arduino-cli: %s", resp.Status)
	}
	var buf bytes.Buffer
	total := resp.ContentLength
	chunk := make([]byte, 64<<10)
	for {
		n, err := resp.Body.Read(chunk)
		buf.Write(chunk[:n])
		if total > 0 {
			progress("downloading arduino-cli", float64(buf.Len())/float64(total))
		} else {
			progress("downloading arduino-cli", -1)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("downloading arduino-cli: %v", err)
		}
	}
	progress("unpacking arduino-cli", -1)
	exeName := "arduino-cli"
	if goos == "windows" {
		exeName += ".exe"
	}
	data, err := unpack(name, buf.Bytes(), exeName)
	if err != nil {
		return "", err
	}
	dir := in.Dir
	if dir == "" {
		dir = Dir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, exeName)
	tmp := dst + ".part"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return dst, nil
}

// unpack takes the tool's file out of a release archive.
func unpack(name string, b []byte, exe string) ([]byte, error) {
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, fmt.Errorf("the download isn't a zip: %v", err)
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == exe {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("no %s in the download", exe)
	}
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("the download isn't a .tar.gz: %v", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == exe && h.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("no %s in the download", exe)
}

// runTool runs arduino-cli (without a console window on Windows).
func runTool(ctx context.Context, exe string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	hideWindow(cmd)
	return cmd.CombinedOutput()
}
