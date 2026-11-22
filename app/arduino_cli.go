package app

// Compiling a sketch (.ino) with arduino-cli, when it is installed: the
// Arduino AVR core and the libraries are the user's own. The compiler runs
// on a goroutine without a console window.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"forma/arduinocli"
)

// arduinoCLI finds arduino-cli: $FORMA_ARDUINO_CLI, the PATH, or where
// its Windows installer puts it.
func arduinoCLI() string { return arduinocli.Find() }

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

// compileMsg is a compiler error or warning on a line of the sketch.
type compileMsg struct {
	Line, Col int // 1-based
	Error     bool
	Text      string
}

var gccMsg = regexp.MustCompile(`\.ino:(\d+):(\d+): (fatal error|error|warning): (.*)$`)

// compileCode builds a sketch's code for the Uno: its Intel HEX, or the
// compiler's messages on the sketch's lines and an error.
func compileCode(name, src string) (string, []compileMsg, error) {
	cli := arduinoCLI()
	if cli == "" {
		return "", nil, fmt.Errorf("arduino-cli isn't installed (or set FORMA_ARDUINO_CLI): install it to compile sketches")
	}
	name = sketchName(name)
	tmp, err := os.MkdirTemp("", "forma-sketch-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(tmp)
	dir := filepath.Join(tmp, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, name+".ino"), []byte(src), 0o644); err != nil {
		return "", nil, err
	}
	out := filepath.Join(tmp, "build")
	cmd := exec.Command(cli, "compile", "--fqbn", "arduino:avr:uno", "--output-dir", out, dir)
	hideConsole(cmd)
	msg, err := cmd.CombinedOutput()
	var msgs []compileMsg
	for _, l := range strings.Split(string(msg), "\n") {
		m := gccMsg.FindStringSubmatch(strings.TrimRight(l, "\r"))
		if m == nil {
			continue
		}
		line, _ := strconv.Atoi(m[1])
		col, _ := strconv.Atoi(m[2])
		msgs = append(msgs, compileMsg{Line: line, Col: col, Error: m[3] != "warning", Text: strings.TrimSpace(m[4])})
	}
	if err != nil {
		for _, m := range msgs {
			if m.Error {
				return "", msgs, fmt.Errorf("line %d: %s", m.Line, m.Text)
			}
		}
		last := strings.TrimSpace(string(msg))
		if i := strings.LastIndex(last, "\n"); i >= 0 {
			last = strings.TrimSpace(last[i+1:])
		}
		return "", msgs, fmt.Errorf("arduino-cli: %s", last)
	}
	hex, err := os.ReadFile(filepath.Join(out, name+".ino.hex"))
	if err != nil {
		return "", msgs, err
	}
	return string(hex), msgs, nil
}

// sketchName is a name arduino-cli takes for a sketch's folder and file.
func sketchName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 || !(b.String()[0] >= 'A' && b.String()[0] <= 'z') {
		return "sketch_" + b.String()
	}
	return b.String()
}
