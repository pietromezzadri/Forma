package app

// Compiling a sketch (.ino) with arduino-cli, when it is installed: the
// Arduino AVR core and the libraries are the user's own. The compiler runs
// on a goroutine without a console window.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// arduinoCLI finds arduino-cli: $FORMA_ARDUINO_CLI, the PATH, or where
// its Windows installer puts it.
func arduinoCLI() string {
	if p := os.Getenv("FORMA_ARDUINO_CLI"); p != "" {
		return p
	}
	if p, err := exec.LookPath("arduino-cli"); err == nil {
		return p
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		p := filepath.Join(la, "arduino-cli", "bin", "arduino-cli.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// compileSketch builds an .ino for the Uno and returns its Intel HEX.
func compileSketch(ino string) (string, error) {
	cli := arduinoCLI()
	if cli == "" {
		return "", fmt.Errorf("arduino-cli isn't installed (or set FORMA_ARDUINO_CLI): compile the sketch yourself and load its .hex")
	}
	dir := filepath.Dir(ino)
	name := strings.TrimSuffix(filepath.Base(ino), filepath.Ext(ino))
	// arduino-cli wants the sketch in a folder of its own name
	if filepath.Base(dir) != name {
		tmp, err := os.MkdirTemp("", "forma-sketch-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmp)
		src, err := os.ReadFile(ino)
		if err != nil {
			return "", err
		}
		dir = filepath.Join(tmp, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, name+".ino"), src, 0o644); err != nil {
			return "", err
		}
	}
	out, err := os.MkdirTemp("", "forma-build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(out)
	cmd := exec.Command(cli, "compile", "--fqbn", "arduino:avr:uno", "--output-dir", out, dir)
	hideConsole(cmd)
	msg, err := cmd.CombinedOutput()
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(msg)), "\n")
		for _, l := range lines {
			if strings.Contains(l, "error") {
				return "", fmt.Errorf("%s", strings.TrimSpace(l))
			}
		}
		return "", fmt.Errorf("arduino-cli: %v", err)
	}
	hex, err := os.ReadFile(filepath.Join(out, name+".ino.hex"))
	if err != nil {
		return "", err
	}
	return string(hex), nil
}
