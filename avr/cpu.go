// Package avr emulates an ATmega328P (an Arduino Uno's or Nano's chip)
// well enough to run firmware compiled for it: the CPU with its whole
// instruction set and cycle counts, the I/O ports, the three timers with
// their PWM modes and interrupts (millis(), analogWrite(), the Servo
// library), the serial port's transmitter and the ADC. The emulator was
// written from Microchip's ATmega328P datasheet and its AVR instruction
// set manual, following the design of avr8js (MIT, Uri Shaked): the CPU
// alone, with peripherals as hooks on its I/O addresses.
package avr

import "fmt"

// Memory map of the ATmega328P's data space.
const (
	ioBase    = 0x20  // the 64 I/O registers (IN / OUT addresses 0..63)
	sramStart = 0x100 // internal SRAM
	DataSize  = 0x900 // registers, I/O, extended I/O and 2 KB of SRAM
	FlashSize = 0x8000

	regSREG = 0x5F
	regSPL  = 0x5D
	regSPH  = 0x5E

	// ClockHz is the Uno's crystal.
	ClockHz = 16_000_000
)

// SREG flags.
const (
	flagC = 1 << iota
	flagZ
	flagN
	flagV
	flagS
	flagH
	flagT
	flagI
)

// CPU is the processor: its program memory, its data space (registers,
// I/O registers and SRAM), the program counter (in words) and the clock.
type CPU struct {
	Prog   []uint16
	Data   []byte
	PC     uint32
	Cycles uint64

	// hooks on I/O addresses (data-space addresses 0x20..0xFF)
	readHook  [256]func(addr int) byte
	writeHook [256]func(addr int, v, old byte) bool // true: the hook stored it

	// interrupts: sources asked in vector order before each instruction
	irq []irqSource

	periph []Peripheral
	// cycles the peripherals haven't been clocked for yet (they run in
	// batches, caught up before any I/O register is touched)
	lag int
	// Sleeping: SLEEP executed; an interrupt wakes it
	Sleeping bool
	// irqHold: the instruction after SEI or RETI runs before any interrupt
	irqHold bool
	// irqDirty: something an interrupt depends on may have changed since
	// none was found pending (an I/O write, a flag, a pin)
	irqDirty bool
	// Break is set by BREAK (a debugger's breakpoint; tests use it to stop)
	Break bool
}

// Peripheral is something clocked with the CPU.
type Peripheral interface {
	Tick(c *CPU, cycles int)
}

type irqSource struct {
	vector  int
	pending func() bool
	taken   func() // clears the request as the CPU takes it (nil: the ISR does)
}

// NewCPU makes a CPU with a flash of the given program (words).
func NewCPU(prog []uint16) *CPU {
	c := &CPU{Prog: make([]uint16, FlashSize/2), Data: make([]byte, DataSize)}
	copy(c.Prog, prog)
	c.Reset()
	return c
}

// Reset starts the program again: registers cleared, the stack at the top
// of SRAM.
func (c *CPU) Reset() {
	for i := range c.Data {
		c.Data[i] = 0
	}
	c.PC, c.Cycles, c.Sleeping, c.Break = 0, 0, false, false
	c.SetSP(DataSize - 1)
}

// Hook puts a reader and / or a writer on a data-space I/O address.
func (c *CPU) Hook(addr int, read func(int) byte, write func(addr int, v, old byte) bool) {
	if read != nil {
		c.readHook[addr] = read
	}
	if write != nil {
		c.writeHook[addr] = write
	}
}

// AddInterrupt registers an interrupt source (vector numbers as in the
// datasheet's table: 1 INT0 … 16 TIMER0_OVF … 25 SPM_READY).
func (c *CPU) AddInterrupt(vector int, pending func() bool, taken func()) {
	c.irq = append(c.irq, irqSource{vector, pending, taken})
	// keep them in priority (vector) order
	for i := len(c.irq) - 1; i > 0 && c.irq[i].vector < c.irq[i-1].vector; i-- {
		c.irq[i], c.irq[i-1] = c.irq[i-1], c.irq[i]
	}
}

// AddPeripheral clocks p with the CPU.
func (c *CPU) AddPeripheral(p Peripheral) { c.periph = append(c.periph, p) }

func (c *CPU) SP() int { return int(c.Data[regSPL]) | int(c.Data[regSPH])<<8 }

func (c *CPU) SetSP(sp int) {
	c.Data[regSPL] = byte(sp)
	c.Data[regSPH] = byte(sp >> 8)
}

func (c *CPU) sreg() byte { return c.Data[regSREG] }

func (c *CPU) flag(f byte) bool { return c.Data[regSREG]&f != 0 }

func (c *CPU) setFlag(f byte, on bool) {
	if on {
		c.Data[regSREG] |= f
	} else {
		c.Data[regSREG] &^= f
	}
}

// Read reads the data space, through any I/O hook.
func (c *CPU) Read(addr int) byte {
	if addr >= ioBase && addr < 0x100 {
		if h := c.readHook[addr]; h != nil {
			c.Sync()
			return h(addr)
		}
	}
	if addr < 0 || addr >= len(c.Data) {
		return 0
	}
	return c.Data[addr]
}

// Write writes the data space, through any I/O hook.
func (c *CPU) Write(addr int, v byte) {
	if addr < 0x100 {
		c.irqDirty = true
	}
	if addr >= ioBase && addr < 0x100 {
		if h := c.writeHook[addr]; h != nil {
			c.Sync()
			if h(addr, v, c.Data[addr]) {
				return
			}
		}
	}
	if addr < 0 || addr >= len(c.Data) {
		return
	}
	c.Data[addr] = v
}

func (c *CPU) push(v byte) {
	sp := c.SP()
	c.Data[sp] = v
	c.SetSP(sp - 1)
}

func (c *CPU) pop() byte {
	sp := c.SP() + 1
	c.SetSP(sp)
	return c.Data[sp]
}

func (c *CPU) pushPC(pc uint32) {
	c.push(byte(pc))
	c.push(byte(pc >> 8))
}

func (c *CPU) popPC() uint32 {
	hi := uint32(c.pop())
	lo := uint32(c.pop())
	return hi<<8 | lo
}

// interrupt takes the highest-priority pending interrupt, if interrupts
// are on: the return address pushed, I cleared, a jump to its vector.
func (c *CPU) interrupt() bool {
	if !c.irqDirty || !c.flag(flagI) {
		return false
	}
	for _, s := range c.irq {
		if s.pending() {
			if s.taken != nil {
				s.taken()
			}
			c.Sleeping = false
			c.pushPC(c.PC)
			c.setFlag(flagI, false)
			c.PC = uint32(s.vector * 2)
			c.tick(4)
			return true
		}
	}
	c.irqDirty = false
	return false
}

// batch is how many cycles the peripherals may fall behind (2 µs): their
// flags and pin edges come that late at most.
const batch = 32

func (c *CPU) tick(n int) {
	c.Cycles += uint64(n)
	c.lag += n
	if c.lag >= batch {
		c.Sync()
	}
}

// Sync clocks the peripherals up to now.
func (c *CPU) Sync() {
	n := c.lag
	c.lag = 0
	if n == 0 {
		return
	}
	for _, p := range c.periph {
		p.Tick(c, n)
	}
}

// Step runs one instruction (or takes an interrupt, or idles a cycle while
// asleep) and returns the cycles it took.
func (c *CPU) Step() int {
	before := c.Cycles
	if c.irqHold {
		c.irqHold = false
	} else if c.interrupt() {
		return int(c.Cycles - before)
	}
	if c.Sleeping {
		// asleep: idle to the next batch of the peripherals' clocks
		n := batch - c.lag
		if n < 1 {
			n = 1
		}
		c.tick(n)
		return n
	}
	n := c.exec()
	c.tick(n)
	return n
}

// RunCycles runs until at least n more cycles have passed.
func (c *CPU) RunCycles(n uint64) {
	end := c.Cycles + n
	for c.Cycles < end && !c.Break {
		c.Step()
	}
}

// RunFor runs for a span of the chip's time (seconds).
func (c *CPU) RunFor(seconds float64) { c.RunCycles(uint64(seconds * ClockHz)) }

// Time is the chip's time since reset, in seconds.
func (c *CPU) Time() float64 { return float64(c.Cycles) / ClockHz }

// Reg is register r (0..31).
func (c *CPU) Reg(r int) byte { return c.Data[r] }

func (c *CPU) String() string {
	return fmt.Sprintf("PC %04x SP %04x SREG %08b cycles %d", c.PC*2, c.SP(), c.sreg(), c.Cycles)
}

// Poke marks that an interrupt may have become pending (a peripheral set
// a flag or a pin changed).
func (c *CPU) Poke() { c.irqDirty = true }
