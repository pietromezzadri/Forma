package avr

// The ATmega328P around its CPU: the three I/O ports, timers 0, 1 and 2,
// the serial port, the ADC and the two external interrupts, wired to the
// data-space addresses and interrupt vectors of the datasheet.

// Pin is a port pin: port B, C or D (0, 1, 2) × 8 + bit.
type Pin int

const (
	PortB = 0
	PortC = 1
	PortD = 2
)

// P makes a pin from a port and a bit.
func P(port, bit int) Pin { return Pin(port*8 + bit) }

func (p Pin) Port() int { return int(p) / 8 }
func (p Pin) Bit() int  { return int(p) % 8 }

func (p Pin) String() string { return string("BCD"[p.Port()]) + string(rune('0'+p.Bit())) }

// Chip is an ATmega328P: the CPU and its peripherals.
type Chip struct {
	*CPU
	Ports  [3]*Port
	Timers [3]*Timer
	USART  *USART
	ADC    *ADC
	ext    *extInt

	// OnPin hears every change of a pin's level (outputs the program
	// drives, a timer's PWM outputs, inputs from outside), with the cycle.
	OnPin func(p Pin, high bool, cycle uint64)
}

// NewChip makes a chip running prog.
func NewChip(prog []uint16) *Chip {
	ch := &Chip{CPU: NewCPU(prog)}
	for i := range ch.Ports {
		ch.Ports[i] = newPort(ch, i)
	}
	ch.Timers[0] = newTimer(ch, timer0)
	ch.Timers[1] = newTimer(ch, timer1)
	ch.Timers[2] = newTimer(ch, timer2)
	ch.USART = newUSART(ch)
	ch.ADC = newADC(ch)
	ch.ext = newExtInt(ch)
	return ch
}

// Level is a pin's level now (its output if the program drives it, else
// what drives it from outside or its pull-up).
func (ch *Chip) Level(p Pin) bool { return ch.Ports[p.Port()].level(p.Bit()) }

// Output reports whether the program drives a pin (its DDR bit set).
func (ch *Chip) Output(p Pin) bool {
	return ch.Data[ch.Ports[p.Port()].ddr]&(1<<p.Bit()) != 0
}

// Drive sets what drives an input pin from outside (a switch, a sensor).
func (ch *Chip) Drive(p Pin, high bool) { ch.Ports[p.Port()].drive(p.Bit(), high) }

// Release lets an input pin float again (its pull-up, if on, holds it).
func (ch *Chip) Release(p Pin) { ch.Ports[p.Port()].release(p.Bit()) }

// ---------------------------------------------------------------- ports

// Port is an I/O port: PINx, DDRx, PORTx.
type Port struct {
	ch            *Chip
	index         int
	pin, ddr, prt int // data-space addresses
	driven, ext   byte
	override      byte // bits a timer's compare output owns
	ovVal         byte
	last          byte // levels as last told
}

func newPort(ch *Chip, i int) *Port {
	base := 0x23 + 3*i
	p := &Port{ch: ch, index: i, pin: base, ddr: base + 1, prt: base + 2}
	ch.Hook(p.pin, func(int) byte { return p.levels() }, func(_ int, v, _ byte) bool {
		// writing a 1 to PINx toggles PORTx
		ch.Data[p.prt] ^= v
		p.changed()
		return true
	})
	ch.Hook(p.ddr, nil, func(_ int, v, _ byte) bool { ch.Data[p.ddr] = v; p.changed(); return true })
	ch.Hook(p.prt, nil, func(_ int, v, _ byte) bool { ch.Data[p.prt] = v; p.changed(); return true })
	return p
}

func (p *Port) level(b int) bool { return p.levels()&(1<<b) != 0 }

// levels are the eight pins' levels.
func (p *Port) levels() byte {
	d := p.ch.Data
	ddr, prt := d[p.ddr], d[p.prt]
	out := prt&^p.override | p.ovVal&p.override
	// inputs: what drives them, else the pull-up (PORTx set)
	in := p.ext&p.driven | prt&^p.driven
	return out&ddr | in&^ddr
}

func (p *Port) changed() {
	now := p.levels()
	diff := now ^ p.last
	p.last = now
	if diff == 0 {
		return
	}
	p.ch.Poke()
	for b := 0; b < 8; b++ {
		if diff&(1<<b) != 0 {
			pin := P(p.index, b)
			p.ch.ext.edge(pin, now&(1<<b) != 0)
			if p.ch.OnPin != nil {
				p.ch.OnPin(pin, now&(1<<b) != 0, p.ch.Cycles)
			}
		}
	}
}

func (p *Port) drive(b int, high bool) {
	p.driven |= 1 << b
	if high {
		p.ext |= 1 << b
	} else {
		p.ext &^= 1 << b
	}
	p.changed()
}

func (p *Port) release(b int) {
	p.driven &^= 1 << b
	p.changed()
}

// setOverride gives bit b to a timer's output (on) or back to PORTx.
func (p *Port) setOverride(b int, on, level bool) {
	if on {
		p.override |= 1 << b
	} else {
		p.override &^= 1 << b
	}
	if level {
		p.ovVal |= 1 << b
	} else {
		p.ovVal &^= 1 << b
	}
	p.changed()
}

// ---------------------------------------------------------------- timers

type timerSpec struct {
	bits                        int // 8 or 16
	tccra, tccrb, tcnt, ocra    int
	ocrb, icr, timsk, tifr      int
	presc                       [8]int
	pinA, pinB                  Pin
	vCapt, vCompA, vCompB, vOvf int
}

var (
	timer0 = timerSpec{bits: 8, tccra: 0x44, tccrb: 0x45, tcnt: 0x46, ocra: 0x47, ocrb: 0x48, timsk: 0x6E, tifr: 0x35,
		presc: [8]int{0, 1, 8, 64, 256, 1024, 0, 0}, pinA: P(PortD, 6), pinB: P(PortD, 5), vCompA: 14, vCompB: 15, vOvf: 16}
	timer1 = timerSpec{bits: 16, tccra: 0x80, tccrb: 0x81, tcnt: 0x84, ocra: 0x88, ocrb: 0x8A, icr: 0x86, timsk: 0x6F, tifr: 0x36,
		presc: [8]int{0, 1, 8, 64, 256, 1024, 0, 0}, pinA: P(PortB, 1), pinB: P(PortB, 2), vCapt: 10, vCompA: 11, vCompB: 12, vOvf: 13}
	timer2 = timerSpec{bits: 8, tccra: 0xB0, tccrb: 0xB1, tcnt: 0xB2, ocra: 0xB3, ocrb: 0xB4, timsk: 0x70, tifr: 0x37,
		presc: [8]int{0, 1, 8, 32, 64, 128, 256, 1024}, pinA: P(PortB, 3), pinB: P(PortD, 3), vCompA: 7, vCompB: 8, vOvf: 9}
)

// TIFR / TIMSK bits
const (
	tov  = 1 << 0
	ocfa = 1 << 1
	ocfb = 1 << 2
	icf  = 1 << 5
)

type timerKind int

const (
	kindNormal timerKind = iota
	kindCTC
	kindFast
	kindPC // phase correct (and phase and frequency correct)
)

// Timer is one of the three timer / counters.
type Timer struct {
	ch              *Chip
	s               timerSpec
	cnt             int
	ocrA, ocrB, icr int
	down            bool
	acc             int
	outA, outB      bool
	temp            byte // the 16-bit registers' shared high byte
}

func newTimer(ch *Chip, s timerSpec) *Timer {
	t := &Timer{ch: ch, s: s}
	ch.AddPeripheral(t)
	reg := func(addr int, get func() int, set func(int)) {
		if s.bits == 8 {
			ch.Hook(addr, func(int) byte { return byte(get()) }, func(_ int, v, _ byte) bool { set(int(v)); return true })
			return
		}
		// 16-bit: the high byte goes through TEMP (written first, read
		// after the low byte)
		ch.Hook(addr, func(int) byte {
			v := get()
			t.temp = byte(v >> 8)
			return byte(v)
		}, func(_ int, v, _ byte) bool {
			set(int(t.temp)<<8 | int(v))
			return true
		})
		ch.Hook(addr+1, func(int) byte { return t.temp }, func(_ int, v, _ byte) bool { t.temp = v; return true })
	}
	reg(s.tcnt, func() int { return t.cnt }, func(v int) { t.cnt = v })
	reg(s.ocra, func() int { return t.ocrA }, func(v int) { t.ocrA = v })
	reg(s.ocrb, func() int { return t.ocrB }, func(v int) { t.ocrB = v })
	if s.icr != 0 {
		reg(s.icr, func() int { return t.icr }, func(v int) { t.icr = v })
	}
	ch.Hook(s.tifr, nil, func(_ int, v, old byte) bool {
		ch.Data[s.tifr] = old &^ v // writing a one clears a flag
		return true
	})
	for _, a := range []int{s.tccra, s.tccrb} {
		ch.Hook(a, nil, func(addr int, v, _ byte) bool {
			ch.Data[addr] = v
			t.outputs()
			return true
		})
	}
	irq := func(vec int, bit byte) {
		if vec == 0 {
			return
		}
		ch.AddInterrupt(vec, func() bool {
			return ch.Data[s.tifr]&bit != 0 && ch.Data[s.timsk]&bit != 0
		}, func() { ch.Data[s.tifr] &^= bit })
	}
	irq(s.vCompA, ocfa)
	irq(s.vCompB, ocfb)
	irq(s.vOvf, tov)
	irq(s.vCapt, icf)
	return t
}

func (t *Timer) wgm() int {
	d := t.ch.Data
	if t.s.bits == 8 {
		return int(d[t.s.tccrb]>>3&1)<<2 | int(d[t.s.tccra]&3)
	}
	return int(d[t.s.tccrb]>>3&3)<<2 | int(d[t.s.tccra]&3)
}

// mode is how the counter runs and its TOP.
func (t *Timer) mode() (timerKind, int) {
	w := t.wgm()
	if t.s.bits == 8 {
		switch w {
		case 1:
			return kindPC, 0xFF
		case 2:
			return kindCTC, t.ocrA
		case 3:
			return kindFast, 0xFF
		case 5:
			return kindPC, t.ocrA
		case 7:
			return kindFast, t.ocrA
		}
		return kindNormal, 0xFF
	}
	switch w {
	case 1:
		return kindPC, 0xFF
	case 2:
		return kindPC, 0x1FF
	case 3:
		return kindPC, 0x3FF
	case 4:
		return kindCTC, t.ocrA
	case 5:
		return kindFast, 0xFF
	case 6:
		return kindFast, 0x1FF
	case 7:
		return kindFast, 0x3FF
	case 8, 10:
		return kindPC, t.icr
	case 9, 11:
		return kindPC, t.ocrA
	case 12:
		return kindCTC, t.icr
	case 14:
		return kindFast, t.icr
	case 15:
		return kindFast, t.ocrA
	}
	return kindNormal, 0xFFFF
}

// Prescale is the CPU cycles per count (0: stopped).
func (t *Timer) Prescale() int { return t.s.presc[t.ch.Data[t.s.tccrb]&7] }

func (t *Timer) Tick(_ *CPU, cycles int) {
	p := t.Prescale()
	if p == 0 {
		return
	}
	t.acc += cycles
	for t.acc >= p {
		t.acc -= p
		t.count()
	}
}

func (t *Timer) flag(bit byte) {
	t.ch.Data[t.s.tifr] |= bit
	t.ch.Poke()
}

func (t *Timer) count() {
	kind, top := t.mode()
	max := 1<<t.s.bits - 1
	switch kind {
	case kindPC:
		if !t.down {
			t.cnt++
			if t.cnt >= top {
				t.cnt = top
				t.down = true
			}
		} else {
			t.cnt--
			if t.cnt <= 0 {
				t.cnt = 0
				t.down = false
				t.flag(tov)
			}
		}
	default:
		if t.cnt == top || t.cnt >= max {
			t.cnt = 0
			if kind != kindCTC || top == max {
				t.flag(tov)
			}
			if kind == kindFast {
				t.atBottom()
			}
		} else {
			t.cnt++
		}
	}
	if t.cnt == t.ocrA {
		t.flag(ocfa)
		t.match(true, kind)
	}
	if t.cnt == t.ocrB {
		t.flag(ocfb)
		t.match(false, kind)
	}
}

func (t *Timer) com(a bool) int {
	v := t.ch.Data[t.s.tccra]
	if a {
		return int(v >> 6 & 3)
	}
	return int(v >> 4 & 3)
}

// match drives a compare output on a compare match.
func (t *Timer) match(a bool, kind timerKind) {
	out := &t.outB
	if a {
		out = &t.outA
	}
	switch c := t.com(a); {
	case c == 0:
		return
	case kind == kindNormal || kind == kindCTC:
		switch c {
		case 1:
			*out = !*out
		case 2:
			*out = false
		case 3:
			*out = true
		}
	case kind == kindFast:
		switch c {
		case 1:
			if a {
				*out = !*out
			}
		case 2:
			*out = false
		case 3:
			*out = true
		}
	default: // phase correct
		switch c {
		case 1:
			if a {
				*out = !*out
			}
		case 2:
			*out = t.down
		case 3:
			*out = !t.down
		}
	}
	t.outputs()
}

// atBottom: fast PWM sets (clears, inverting) its outputs at BOTTOM.
func (t *Timer) atBottom() {
	if c := t.com(true); c == 2 || c == 3 {
		t.outA = c == 2
	}
	if c := t.com(false); c == 2 || c == 3 {
		t.outB = c == 2
	}
	t.outputs()
}

// outputs gives the compare output pins to the timer while its COM bits
// are set.
func (t *Timer) outputs() {
	set := func(p Pin, com int, level bool) {
		t.ch.Ports[p.Port()].setOverride(p.Bit(), com != 0, level)
	}
	set(t.s.pinA, t.com(true), t.outA)
	set(t.s.pinB, t.com(false), t.outB)
}

// ---------------------------------------------------------------- USART

// USART is the serial port. Bytes the program sends arrive at OnTX (and
// in Sent); Receive feeds it bytes.
type USART struct {
	ch   *Chip
	OnTX func(b byte)
	Sent []byte
	rx   []byte
}

const (
	ucsr0a = 0xC0
	ucsr0b = 0xC1
	udr0   = 0xC6
	rxc    = 1 << 7
	txc    = 1 << 6
	udre   = 1 << 5
)

func newUSART(ch *Chip) *USART {
	u := &USART{ch: ch}
	ch.Data[ucsr0a] = udre
	ch.Hook(udr0, func(int) byte {
		if len(u.rx) == 0 {
			return 0
		}
		b := u.rx[0]
		u.rx = u.rx[1:]
		if len(u.rx) == 0 {
			ch.Data[ucsr0a] &^= rxc
		}
		return b
	}, func(_ int, v, _ byte) bool {
		if ch.Data[ucsr0b]&(1<<3) != 0 { // TXEN
			u.Sent = append(u.Sent, v)
			if u.OnTX != nil {
				u.OnTX(v)
			}
			ch.Data[ucsr0a] |= txc | udre
		}
		return true
	})
	ch.Hook(ucsr0a, nil, func(_ int, v, old byte) bool {
		// TXC clears by writing a one; RXC and UDRE are read-only
		ch.Data[ucsr0a] = old&(rxc|udre) | old&txc&^v | v&0x03
		return true
	})
	b := func() byte { return ch.Data[ucsr0b] }
	ch.AddInterrupt(18, func() bool { return ch.Data[ucsr0a]&rxc != 0 && b()&(1<<7) != 0 }, nil)
	ch.AddInterrupt(19, func() bool { return ch.Data[ucsr0a]&udre != 0 && b()&(1<<5) != 0 }, nil)
	ch.AddInterrupt(20, func() bool { return ch.Data[ucsr0a]&txc != 0 && b()&(1<<6) != 0 },
		func() { ch.Data[ucsr0a] &^= txc })
	return u
}

// Receive puts bytes in the serial port's receive buffer.
func (u *USART) Receive(b ...byte) {
	u.rx = append(u.rx, b...)
	u.ch.Poke()
	if len(u.rx) > 0 {
		u.ch.Data[ucsr0a] |= rxc
	}
}

// ---------------------------------------------------------------- ADC

// ADC is the analogue-to-digital converter. Volts gives each channel's
// voltage (0..5 V against the 5 V reference).
type ADC struct {
	ch    *Chip
	Volts func(channel int) float64
	left  int // cycles until the conversion ends
}

const (
	adcl   = 0x78
	adch   = 0x79
	adcsra = 0x7A
	admux  = 0x7C
	adsc   = 1 << 6
	adif   = 1 << 4
)

func newADC(ch *Chip) *ADC {
	a := &ADC{ch: ch}
	ch.AddPeripheral(a)
	ch.Hook(adcsra, nil, func(_ int, v, old byte) bool {
		v = v&^adif | old&adif&^(v&adif) // ADIF clears by writing a one
		if v&adsc != 0 && old&adsc == 0 && v&0x80 != 0 {
			pre := 1 << (v & 7)
			if pre < 2 {
				pre = 2
			}
			a.left = 13 * pre
		}
		ch.Data[adcsra] = v
		return true
	})
	ch.AddInterrupt(21, func() bool {
		return ch.Data[adcsra]&adif != 0 && ch.Data[adcsra]&(1<<3) != 0
	}, func() { ch.Data[adcsra] &^= adif })
	return a
}

func (a *ADC) Tick(_ *CPU, cycles int) {
	if a.left <= 0 {
		return
	}
	a.left -= cycles
	if a.left > 0 {
		return
	}
	d := a.ch.Data
	chn := int(d[admux] & 0x0F)
	v := 0.0
	if a.Volts != nil {
		v = a.Volts(chn)
	}
	n := int(v/5*1023 + 0.5)
	if n < 0 {
		n = 0
	}
	if n > 1023 {
		n = 1023
	}
	if d[admux]&(1<<5) != 0 { // ADLAR: left adjusted
		n <<= 6
	}
	d[adcl], d[adch] = byte(n), byte(n>>8)
	d[adcsra] = d[adcsra]&^adsc | adif
	a.ch.Poke()
}

// ---------------------------------------------------------------- INT0 / INT1

type extInt struct{ ch *Chip }

const (
	eicra = 0x69
	eimsk = 0x3D
	eifr  = 0x3C
)

func newExtInt(ch *Chip) *extInt {
	e := &extInt{ch: ch}
	ch.Hook(eifr, nil, func(_ int, v, old byte) bool { ch.Data[eifr] = old &^ v; return true })
	for i := 0; i < 2; i++ {
		bit := byte(1 << i)
		pin := P(PortD, 2+i)
		ch.AddInterrupt(1+i, func() bool {
			if ch.Data[eimsk]&bit == 0 {
				return false
			}
			if ch.Data[eicra]>>(2*i)&3 == 0 { // low level: while it is low
				return !ch.Level(pin)
			}
			return ch.Data[eifr]&bit != 0
		}, func() { ch.Data[eifr] &^= bit })
	}
	return e
}

// edge flags INT0 / INT1 on their pins' edges as EICRA asks.
func (e *extInt) edge(p Pin, high bool) {
	if p.Port() != PortD || (p.Bit() != 2 && p.Bit() != 3) {
		return
	}
	i := p.Bit() - 2
	mode := e.ch.Data[eicra] >> (2 * i) & 3
	if mode == 1 || mode == 2 && !high || mode == 3 && high {
		e.ch.Data[eifr] |= 1 << i
	}
}
