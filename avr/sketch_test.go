package avr

import (
	"strconv"
	"strings"
	"testing"
)

// Sketches compiled by arduino-cli (arduino:avr 1.8.8) from
// testdata/sketches; their .hex files are kept beside them, so the tests
// need no compiler.

// blink: the Arduino core's start-up, millis(), delay(), micros() and
// Serial.
func TestSketchBlink(t *testing.T) {
	uno, err := LoadUno("testdata/blink.hex")
	if err != nil {
		t.Fatal(err)
	}
	toggles := 0
	uno.OnPin = func(p Pin, _ bool, _ uint64) {
		if p == D(13) {
			toggles++
		}
	}
	uno.RunFor(1.05)
	lines := strings.Split(strings.TrimSpace(string(uno.USART.Sent)), "\r\n")
	t.Logf("serial: %q, %d toggles", lines, toggles)
	if len(lines) < 3 || lines[0] != "hello" {
		t.Fatalf("serial %q", uno.USART.Sent)
	}
	// millis() counts 1.024 ms overflows (plus one every ~42): 48 have passed
	// when delay(50) returns at 50.1 ms, so it reads 49, as on a real board
	if ms, err := strconv.Atoi(lines[1]); err != nil || ms < 49 || ms > 51 {
		t.Fatalf("millis() after delay(50) printed %q", lines[1])
	}
	us, err := strconv.Atoi(strings.TrimPrefix(lines[2], "ten at "))
	if err != nil || us < 1_000_000 || us > 1_002_000 {
		t.Fatalf("micros() at the tenth toggle printed %q", lines[2])
	}
	if toggles != 10 {
		t.Fatalf("%d toggles of D13 in 1.05 s, want 10", toggles)
	}
}
