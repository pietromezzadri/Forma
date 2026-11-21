package elec

import (
	"math"
	"testing"

	"forma/avr"
)

// Instruction encoders for small test programs.
func ldi(d int, k byte) uint16 {
	return 0xE000 | uint16(k&0xF0)<<4 | uint16(d-16)<<4 | uint16(k&0x0F)
}
func out(a, r int) uint16 {
	return 0xB800 | uint16(a&0x30)<<5 | uint16(r)<<4 | uint16(a&0x0F)
}
func sts(addr, r int) []uint16 { return []uint16{0x9200 | uint16(r)<<4, uint16(addr)} }
func sbi(a, b int) uint16      { return 0x9A00 | uint16(a)<<3 | uint16(b) }
func cbi(a, b int) uint16      { return 0x9800 | uint16(a)<<3 | uint16(b) }
func sbis(a, b int) uint16     { return 0x9B00 | uint16(a)<<3 | uint16(b) }
func rjmp(k int) uint16        { return 0xC000 | uint16(k)&0x0FFF }

func prog(parts ...any) []uint16 {
	var w []uint16
	for _, p := range parts {
		switch v := p.(type) {
		case uint16:
			w = append(w, v)
		case []uint16:
			w = append(w, v...)
		}
	}
	return w
}

// stsReg writes an immediate to a data-space register through r16.
func stsReg(addr int, v byte) []uint16 { return prog(ldi(16, v), sts(addr, 16)) }

// A servo on D9 fed by Timer 1 in fast PWM at 50 Hz with 1.5 ms pulses
// turns to the Servo library's angle for 1500 µs.
func TestServoFromTimer1(t *testing.T) {
	uno := avr.NewUno(prog(
		sbi(0x04, 1),                           // DDRB1 (D9) out
		stsReg(0x80, 0x82),                     // TCCR1A: COM1A1, WGM11
		stsReg(0x87, 0x9C), stsReg(0x86, 0x3F), // ICR1 = 39999: 20 ms at /8
		stsReg(0x89, 0x0B), stsReg(0x88, 0xB7), // OCR1A = 2999: 1.5 ms
		stsReg(0x81, 0x1A), // TCCR1B: WGM13, WGM12, /8
		rjmp(-1),
	))
	servo := SG90("steering", avr.D(9))
	servo.Angle = 0
	s := New(uno, servo)
	s.Run(0.5, 0.001)
	want := (1500 - 544.0) / (2400 - 544) * 180
	if math.Abs(servo.Angle-want) > 0.5 {
		t.Fatalf("servo at %.2f°, want %.2f° (%s)", servo.Angle, want, s.Report())
	}
}

// 50 % PWM from Timer 0 on a TB6612's enable, IN1 high, IN2 low: a TT
// motor on 6 V runs at half its no-load speed less the bridge's drop.
func TestMotorFromPWM(t *testing.T) {
	uno := avr.NewUno(prog(
		sbi(0x0A, 6), sbi(0x0A, 7), sbi(0x04, 0), // D6, D7, D8 out
		sbi(0x0B, 7),                 // IN1 (D7) high
		ldi(16, 0x83), out(0x24, 16), // TCCR0A: COM0A1, fast PWM
		ldi(16, 127), out(0x27, 16), // OCR0A: half
		ldi(16, 0x03), out(0x25, 16), // /64
		rjmp(-1),
	))
	bat := &Battery{Name: "2S pack", Volts: 6}
	m := TTMotor("rear")
	drv := &Driver{Name: "TB6612", In1: avr.D(7), In2: avr.D(8), En: avr.D(6), Supply: bat, Drop: 0.1, Motor: m}
	s := New(uno, bat, drv, m)
	s.Run(1, 0.001)
	want := 0.5 * (6 - 0.1) / 6 * 200
	if math.Abs(m.RPM()-want) > 0.03*want {
		t.Fatalf("%.1f rpm, want about %.1f (%s)", m.RPM(), want, s.Report())
	}
	if m.Angle <= 0 {
		t.Fatal("the motor turned backwards")
	}
	// stopped: the motor coasts down
	drv.En = NoPin
	drv.In1 = avr.D(8)
	s.Run(1, 0.001)
	if math.Abs(m.RPM()) > 1 {
		t.Fatalf("with both inputs low it still turns at %.1f rpm", m.RPM())
	}
}

// A button on D2 (pull-up, pressed reads LOW) copied to the LED on D13.
func TestButtonAndLED(t *testing.T) {
	uno := avr.NewUno(prog(
		sbi(0x04, 5), // DDRB5 (D13) out
		sbi(0x0B, 2), // PORTD2: D2's pull-up
		// loop: if D2 is high, LED on, else off
		sbis(0x09, 2), rjmp(2), sbi(0x05, 5), rjmp(-4),
		cbi(0x05, 5), rjmp(-6),
	))
	btn := &Button{Name: "button", Pin: avr.D(2)}
	led := &LED{Name: "LED", Pin: avr.D(13)}
	s := New(uno, btn, led)
	s.Run(0.01, 0.001)
	if led.Brightness < 0.99 {
		t.Fatalf("released: LED at %.2f", led.Brightness)
	}
	btn.Pressed = true
	s.Run(0.01, 0.001)
	if led.Brightness > 0.01 {
		t.Fatalf("pressed: LED at %.2f", led.Brightness)
	}
}

// A pot on A0 read through the ADC.
func TestPot(t *testing.T) {
	uno := avr.NewUno(prog(
		stsReg(0x7C, 0x40), // ADMUX: AVcc, A0
		stsReg(0x7A, 0xC7), // start a conversion
		rjmp(-1),
	))
	pot := &Pot{Name: "throttle", Channel: 0, Value: 0.25}
	s := New(uno, pot)
	s.Run(0.01, 0.001)
	if v := int(uno.Data[0x78]) | int(uno.Data[0x79])<<8; v != 256 {
		t.Fatalf("ADC read %d, want 256", v)
	}
}
