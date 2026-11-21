package avr

import (
	"math"
	"strings"
	"testing"
)

// A few instruction encoders, enough to write test programs by hand.
func ldi(d int, k byte) uint16 {
	return 0xE000 | uint16(k&0xF0)<<4 | uint16(d-16)<<4 | uint16(k&0x0F)
}
func rr(base uint16, d, r int) uint16 {
	return base | uint16(r&0x10)<<5 | uint16(d)<<4 | uint16(r&0x0F)
}
func add(d, r int) uint16  { return rr(0x0C00, d, r) }
func adc(d, r int) uint16  { return rr(0x1C00, d, r) }
func sub(d, r int) uint16  { return rr(0x1800, d, r) }
func mul(d, r int) uint16  { return rr(0x9C00, d, r) }
func movr(d, r int) uint16 { return rr(0x2C00, d, r) }
func imm(base uint16, d int, k byte) uint16 {
	return base | uint16(k&0xF0)<<4 | uint16(d-16)<<4 | uint16(k&0x0F)
}
func subi(d int, k byte) uint16 { return imm(0x5000, d, k) }
func sbci(d int, k byte) uint16 { return imm(0x4000, d, k) }
func out(a, r int) uint16 {
	return 0xB800 | uint16(a&0x30)<<5 | uint16(r)<<4 | uint16(a&0x0F)
}
func in(r, a int) uint16 {
	return 0xB000 | uint16(a&0x30)<<5 | uint16(r)<<4 | uint16(a&0x0F)
}
func sts(addr int, r int) []uint16 { return []uint16{0x9200 | uint16(r)<<4, uint16(addr)} }
func lds(r int, addr int) []uint16 { return []uint16{0x9000 | uint16(r)<<4, uint16(addr)} }
func rjmp(k int) uint16            { return 0xC000 | uint16(k)&0x0FFF }
func rcall(k int) uint16           { return 0xD000 | uint16(k)&0x0FFF }
func brne(k int) uint16            { return 0xF401 | (uint16(k)&0x7F)<<3 }
func inc(d int) uint16             { return 0x9403 | uint16(d)<<4 }
func dec(d int) uint16             { return 0x940A | uint16(d)<<4 }
func push(r int) uint16            { return 0x920F | uint16(r)<<4 }
func pop(r int) uint16             { return 0x900F | uint16(r)<<4 }
func sbi(a, b int) uint16          { return 0x9A00 | uint16(a)<<3 | uint16(b) }
func sbrc(r, b int) uint16         { return 0xFC00 | uint16(r)<<4 | uint16(b) }
func adiw(d int, k int) uint16 {
	return 0x9600 | uint16(k&0x30)<<2 | uint16((d-24)/2)<<4 | uint16(k&0x0F)
}
func sbiw(d int, k int) uint16 { return adiw(d, k) | 0x0100 }

const (
	sei  = 0x9478
	ret  = 0x9508
	reti = 0x9518
	brk  = 0x9598
	nop  = 0x0000
)

func prog(parts ...any) []uint16 {
	var out []uint16
	for _, p := range parts {
		switch v := p.(type) {
		case uint16:
			out = append(out, v)
		case int:
			out = append(out, uint16(v))
		case []uint16:
			out = append(out, v...)
		}
	}
	return out
}

func run(t *testing.T, p []uint16) *Chip {
	t.Helper()
	ch := NewChip(p)
	for i := 0; i < 100000 && !ch.Break; i++ {
		ch.Step()
	}
	if !ch.Break {
		t.Fatal("the program didn't reach BREAK")
	}
	return ch
}

func TestArithmeticFlags(t *testing.T) {
	all := byte(flagC | flagZ | flagN | flagV | flagS | flagH)
	cases := []struct {
		name  string
		ins   uint16
		a, b  byte
		res   byte
		flags byte
	}{
		{"add carry", add(16, 17), 0xFF, 0x01, 0x00, flagC | flagZ | flagH},
		{"add overflow", add(16, 17), 0x7F, 0x01, 0x80, flagN | flagV | flagH},
		{"sub borrow", sub(16, 17), 0x00, 0x01, 0xFF, flagC | flagN | flagS | flagH},
		{"sub overflow", sub(16, 17), 0x80, 0x01, 0x7F, flagV | flagS | flagH},
	}
	for _, c := range cases {
		ch := run(t, prog(ldi(16, c.a), ldi(17, c.b), c.ins, brk))
		if ch.Reg(16) != c.res {
			t.Fatalf("%s: %02x, want %02x", c.name, ch.Reg(16), c.res)
		}
		if got := ch.sreg() & all; got != c.flags {
			t.Fatalf("%s: flags %08b, want %08b", c.name, got, c.flags)
		}
	}
	// 16-bit: 0x01FF + 1 with add / adc; 0x0100 - 1 with subi / sbci,
	// whose Z stays clear because the low byte isn't zero
	ch := run(t, prog(ldi(24, 0xFF), ldi(25, 0x01), ldi(16, 1), ldi(17, 0),
		add(24, 16), adc(25, 17), brk))
	if ch.Reg(24) != 0 || ch.Reg(25) != 2 {
		t.Fatalf("16-bit add: %02x%02x", ch.Reg(25), ch.Reg(24))
	}
	ch = run(t, prog(ldi(24, 0x00), ldi(25, 0x01), subi(24, 1), sbci(25, 0), brk))
	if ch.Reg(24) != 0xFF || ch.Reg(25) != 0 || ch.flag(flagZ) {
		t.Fatalf("16-bit sub: %02x%02x Z %v", ch.Reg(25), ch.Reg(24), ch.flag(flagZ))
	}
	ch = run(t, prog(ldi(16, 200), ldi(17, 3), mul(16, 17), brk))
	if ch.word(0) != 600 {
		t.Fatalf("mul: %d", ch.word(0))
	}
	ch = run(t, prog(ldi(24, 0xFE), ldi(25, 0x00), adiw(24, 5), movr(2, 24), movr(3, 25),
		sbiw(24, 60), brk))
	if ch.word(2) != 0x103 || ch.word(24) != 0x103-60 {
		t.Fatalf("adiw %04x sbiw %04x", ch.word(2), ch.word(24))
	}
}

// A count-down loop: the branches and the cycles they take.
func TestLoopCycles(t *testing.T) {
	ch := run(t, prog(ldi(16, 10), dec(16), brne(-2), brk))
	// ldi 1 + 10 dec + 9 taken branches (2) + 1 not taken (1) + break 1
	if ch.Cycles != 31 {
		t.Fatalf("%d cycles, want 31", ch.Cycles)
	}
}

// Calls, returns and the stack.
func TestCallsAndStack(t *testing.T) {
	ch := run(t, prog(
		ldi(16, 7), push(16), rcall(2), pop(17), brk,
		inc(16), ret,
	))
	if ch.Reg(16) != 8 || ch.Reg(17) != 7 || ch.SP() != DataSize-1 {
		t.Fatalf("r16 %d r17 %d SP %x", ch.Reg(16), ch.Reg(17), ch.SP())
	}
}

// Timer 0 in fast PWM at /64, as the Arduino core sets it up for millis():
// an overflow every 256 × 64 cycles, counted by its interrupt.
func TestTimer0Overflow(t *testing.T) {
	p := make([]uint16, 34)
	p[0] = rjmp(33)  // reset → main at word 34
	p[32] = rjmp(32) // vector 16 (TIMER0_OVF) at word 32 → the ISR at 65
	p = append(p, prog(
		ldi(16, 0x03), out(0x24, 16), // TCCR0A: fast PWM
		ldi(16, 0x03), out(0x25, 16), // TCCR0B: /64
		ldi(16, 0x01), sts(0x6E, 16), // TIMSK0: the overflow interrupt
		sei, rjmp(-1),
	)...)
	for len(p) < 65 {
		p = append(p, nop)
	}
	p = append(p, inc(20), reti)
	ch := NewChip(p)
	ch.RunFor(0.1)
	secs := 0.1
	want := int(secs * ClockHz / (256 * 64))
	if got := int(ch.Reg(20)); got < want-1 || got > want+1 {
		t.Fatalf("%d overflows in 0.1 s, want %d", got, want)
	}
}

// Fast PWM on OC0A (D6): the pin is high for (OCR0A + 1) / 256 of the time.
func TestPWMDuty(t *testing.T) {
	ch := NewChip(prog(
		sbi(0x0A, 6),                 // DDRD6: an output
		ldi(16, 0x83), out(0x24, 16), // COM0A1, fast PWM
		ldi(16, 63), out(0x27, 16), // OCR0A = 63
		ldi(16, 0x03), out(0x25, 16), // /64
		rjmp(-1),
	))
	pin := P(PortD, 6)
	var high, since uint64
	on := false
	ch.OnPin = func(p Pin, h bool, cyc uint64) {
		if p != pin {
			return
		}
		if on {
			high += cyc - since
		}
		on, since = h, cyc
	}
	ch.RunFor(0.05)
	duty := float64(high) / float64(ch.Cycles)
	if math.Abs(duty-64.0/256) > 0.01 {
		t.Fatalf("duty %.4f, want %.4f", duty, 64.0/256)
	}
}

// Ports: SBI on DDRB and PORTB drives D13; an input reads what drives it.
func TestPorts(t *testing.T) {
	var edges []string
	ch := NewChip(prog(sbi(0x04, 5), sbi(0x05, 5), in(20, 0x09), brk))
	ch.OnPin = func(p Pin, h bool, _ uint64) {
		if h {
			edges = append(edges, UnoName(p)+" high")
		}
	}
	ch.Drive(P(PortD, 2), true)
	for !ch.Break {
		ch.Step()
	}
	if !ch.Level(D(13)) || len(edges) < 2 || edges[len(edges)-1] != "D13 high" {
		t.Fatalf("D13 %v, edges %v", ch.Level(D(13)), edges)
	}
	if ch.Reg(20)&0x04 == 0 {
		t.Fatalf("PIND %08b: D2 driven high reads low", ch.Reg(20))
	}
}

// The serial port sends what is written to UDR0.
func TestSerial(t *testing.T) {
	ch := run(t, prog(
		ldi(16, 1<<3), sts(0xC1, 16), // TXEN0
		ldi(16, 'O'), sts(0xC6, 16),
		ldi(16, 'K'), sts(0xC6, 16),
		brk,
	))
	if string(ch.USART.Sent) != "OK" {
		t.Fatalf("sent %q", ch.USART.Sent)
	}
}

// The ADC converts a channel's voltage (2.5 V → 512 of 1023).
func TestADC(t *testing.T) {
	ch := NewChip(prog(
		ldi(16, 0x41), sts(0x7C, 16), // ADMUX: AVcc, channel 1
		ldi(16, 0xC7), sts(0x7A, 16), // ADEN | ADSC, /128
		lds(16, 0x7A), sbrc(16, 6), rjmp(-4),
		lds(24, 0x78), lds(25, 0x79),
		brk,
	))
	ch.ADC.Volts = func(c int) float64 {
		if c == 1 {
			return 2.5
		}
		return 0
	}
	for i := 0; i < 1e6 && !ch.Break; i++ {
		ch.Step()
	}
	if v := ch.word(24); v != 512 {
		t.Fatalf("ADC %d, want 512", v)
	}
	if ch.Cycles < 13*128 {
		t.Fatalf("converted in %d cycles", ch.Cycles)
	}
}

func TestParseHex(t *testing.T) {
	src := ":100000000C9434000C943E000C943E000C943E0082\n:00000001FF\n"
	w, err := ParseHex(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 8 || w[0] != 0x940C || w[1] != 0x0034 {
		t.Fatalf("%04x", w)
	}
	if _, err := ParseHex(strings.NewReader(":100000000C9434000C943E000C943E000C943E0083\n")); err == nil {
		t.Fatal("a bad checksum passed")
	}
}
