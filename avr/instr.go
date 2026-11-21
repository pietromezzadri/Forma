package avr

// The instruction set of the AVRe+ core (ATmega328P): every instruction
// it has, with the cycle counts of the instruction set manual.

func b2i(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// is32 reports an instruction two words long (the next one, for skips).
func is32(op uint16) bool {
	return op&0xFE0F == 0x9000 || op&0xFE0F == 0x9200 || // LDS, STS
		op&0xFE0E == 0x940C || op&0xFE0E == 0x940E // JMP, CALL
}

// skip moves past the next instruction, returning the cycles it took.
func (c *CPU) skip() int {
	if is32(c.Prog[c.PC&0x3FFF]) {
		c.PC += 2
		return 2
	}
	c.PC++
	return 1
}

func (c *CPU) fetch() uint16 {
	op := c.Prog[c.PC&0x3FFF]
	c.PC++
	return op
}

func (c *CPU) setZNS(r byte) {
	c.setFlag(flagZ, r == 0)
	c.setFlag(flagN, r&0x80 != 0)
	c.setFlag(flagS, c.flag(flagN) != c.flag(flagV))
}

func (c *CPU) flagsAdd(d, r, res byte) {
	h := (d&r | r&^res | ^res&d) & 0x08
	cy := (d&r | r&^res | ^res&d) & 0x80
	c.setFlag(flagH, h != 0)
	c.setFlag(flagC, cy != 0)
	c.setFlag(flagV, (d&r&^res|^d&^r&res)&0x80 != 0)
	c.setZNS(res)
}

// flagsSub sets the flags of d − r (− carry) = res; keepZ for SBC, SBCI
// and CPC, whose Z only stays set.
func (c *CPU) flagsSub(d, r, res byte, keepZ bool) {
	h := (^d&r | r&res | res&^d) & 0x08
	cy := (^d&r | r&res | res&^d) & 0x80
	c.setFlag(flagH, h != 0)
	c.setFlag(flagC, cy != 0)
	c.setFlag(flagV, (d&^r&^res|^d&r&res)&0x80 != 0)
	c.setFlag(flagN, res&0x80 != 0)
	if keepZ {
		c.setFlag(flagZ, res == 0 && c.flag(flagZ))
	} else {
		c.setFlag(flagZ, res == 0)
	}
	c.setFlag(flagS, c.flag(flagN) != c.flag(flagV))
}

func (c *CPU) flagsLogic(res byte) {
	c.setFlag(flagV, false)
	c.setZNS(res)
}

func (c *CPU) word(r int) int { return int(c.Data[r]) | int(c.Data[r+1])<<8 }

func (c *CPU) setWord(r int, v int) {
	c.Data[r] = byte(v)
	c.Data[r+1] = byte(v >> 8)
}

// exec runs the instruction at PC and returns its cycles.
func (c *CPU) exec() int {
	op := c.fetch()
	d5 := int(op>>4) & 0x1F
	r5 := int(op&0x0F) | int(op>>5)&0x10
	d4 := 16 + int(op>>4)&0x0F
	k8 := byte(op&0x0F | op>>4&0xF0)

	switch op >> 12 {
	case 0x0:
		switch {
		case op == 0:
			return 1 // NOP
		case op&0xFF00 == 0x0100: // MOVW
			d, r := int(op>>4&0xF)*2, int(op&0xF)*2
			c.Data[d], c.Data[d+1] = c.Data[r], c.Data[r+1]
			return 1
		case op&0xFF00 == 0x0200: // MULS
			p := int(int8(c.Data[d4])) * int(int8(c.Data[16+int(op&0xF)]))
			c.mulResult(p, false)
			return 2
		case op&0xFF88 == 0x0300: // MULSU
			p := int(int8(c.Data[16+int(op>>4&7)])) * int(c.Data[16+int(op&7)])
			c.mulResult(p, false)
			return 2
		case op&0xFF88 == 0x0308: // FMUL
			p := int(c.Data[16+int(op>>4&7)]) * int(c.Data[16+int(op&7)])
			c.mulResult(p, true)
			return 2
		case op&0xFF88 == 0x0380: // FMULS
			p := int(int8(c.Data[16+int(op>>4&7)])) * int(int8(c.Data[16+int(op&7)]))
			c.mulResult(p, true)
			return 2
		case op&0xFF88 == 0x0388: // FMULSU
			p := int(int8(c.Data[16+int(op>>4&7)])) * int(c.Data[16+int(op&7)])
			c.mulResult(p, true)
			return 2
		case op&0xFC00 == 0x0400: // CPC
			d, r := c.Data[d5], c.Data[r5]
			c.flagsSub(d, r, d-r-b2i(c.flag(flagC)), true)
			return 1
		case op&0xFC00 == 0x0800: // SBC
			d, r := c.Data[d5], c.Data[r5]
			res := d - r - b2i(c.flag(flagC))
			c.Data[d5] = res
			c.flagsSub(d, r, res, true)
			return 1
		case op&0xFC00 == 0x0C00: // ADD
			d, r := c.Data[d5], c.Data[r5]
			res := d + r
			c.Data[d5] = res
			c.flagsAdd(d, r, res)
			return 1
		}
	case 0x1:
		d, r := c.Data[d5], c.Data[r5]
		switch op & 0xFC00 {
		case 0x1000: // CPSE
			if d == r {
				return 1 + c.skip()
			}
			return 1
		case 0x1400: // CP
			c.flagsSub(d, r, d-r, false)
			return 1
		case 0x1800: // SUB
			res := d - r
			c.Data[d5] = res
			c.flagsSub(d, r, res, false)
			return 1
		case 0x1C00: // ADC
			res := d + r + b2i(c.flag(flagC))
			c.Data[d5] = res
			c.flagsAdd(d, r, res)
			return 1
		}
	case 0x2:
		d, r := c.Data[d5], c.Data[r5]
		var res byte
		switch op & 0xFC00 {
		case 0x2000: // AND
			res = d & r
		case 0x2400: // EOR
			res = d ^ r
		case 0x2800: // OR
			res = d | r
		case 0x2C00: // MOV
			c.Data[d5] = r
			return 1
		}
		c.Data[d5] = res
		c.flagsLogic(res)
		return 1
	case 0x3: // CPI
		d := c.Data[d4]
		c.flagsSub(d, k8, d-k8, false)
		return 1
	case 0x4: // SBCI
		d := c.Data[d4]
		res := d - k8 - b2i(c.flag(flagC))
		c.Data[d4] = res
		c.flagsSub(d, k8, res, true)
		return 1
	case 0x5: // SUBI
		d := c.Data[d4]
		res := d - k8
		c.Data[d4] = res
		c.flagsSub(d, k8, res, false)
		return 1
	case 0x6: // ORI
		res := c.Data[d4] | k8
		c.Data[d4] = res
		c.flagsLogic(res)
		return 1
	case 0x7: // ANDI
		res := c.Data[d4] & k8
		c.Data[d4] = res
		c.flagsLogic(res)
		return 1
	case 0x8, 0xA: // LDD / STD (Y+q, Z+q)
		q := int(op&7) | int(op>>7)&0x18 | int(op>>8)&0x20
		base := 30
		if op&0x08 != 0 {
			base = 28
		}
		addr := c.word(base) + q
		if op&0x0200 != 0 {
			c.Write(addr, c.Data[d5])
		} else {
			c.Data[d5] = c.Read(addr)
		}
		return 2
	case 0x9:
		return c.exec9(op, d5, r5)
	case 0xB:
		a := ioBase + int(op&0x0F|op>>5&0x30)
		if op&0x0800 != 0 { // OUT
			c.Write(a, c.Data[d5])
		} else { // IN
			c.Data[d5] = c.Read(a)
		}
		return 1
	case 0xC: // RJMP
		c.PC = uint32(int32(c.PC) + int32(int16(op<<4)>>4))
		return 2
	case 0xD: // RCALL
		c.pushPC(c.PC)
		c.PC = uint32(int32(c.PC) + int32(int16(op<<4)>>4))
		return 3
	case 0xE: // LDI
		c.Data[d4] = k8
		return 1
	case 0xF:
		b := op & 7
		switch {
		case op&0x0800 == 0: // BRBS / BRBC
			set := c.Data[regSREG]&(1<<b) != 0
			if set == (op&0x0400 == 0) {
				c.PC = uint32(int32(c.PC) + int32(int16(op<<6)>>9))
				return 2
			}
			return 1
		case op&0xFE08 == 0xF800: // BLD
			if c.flag(flagT) {
				c.Data[d5] |= 1 << b
			} else {
				c.Data[d5] &^= 1 << b
			}
			return 1
		case op&0xFE08 == 0xFA00: // BST
			c.setFlag(flagT, c.Data[d5]&(1<<b) != 0)
			return 1
		case op&0xFE08 == 0xFC00: // SBRC
			if c.Data[d5]&(1<<b) == 0 {
				return 1 + c.skip()
			}
			return 1
		case op&0xFE08 == 0xFE00: // SBRS
			if c.Data[d5]&(1<<b) != 0 {
				return 1 + c.skip()
			}
			return 1
		}
	}
	return 1 // an unknown opcode: a NOP
}

// mulResult puts a product in R1:R0 (shifted for the fractional ones).
func (c *CPU) mulResult(p int, frac bool) {
	u := uint16(p)
	c.setFlag(flagC, u&0x8000 != 0)
	if frac {
		u <<= 1
	}
	c.setWord(0, int(u))
	c.setFlag(flagZ, u == 0)
}

func (c *CPU) exec9(op uint16, d5, r5 int) int {
	switch {
	case op&0xFE00 == 0x9000: // loads
		switch op & 0x0F {
		case 0x0: // LDS
			c.Data[d5] = c.Read(int(c.fetch()))
			return 2
		case 0x1, 0x2: // LD Z+, -Z
			c.Data[d5] = c.loadPtr(30, op&0x0F == 1, op&0x0F == 2)
			return 2
		case 0x4, 0x5: // LPM Rd, Z(+)
			z := c.word(30)
			c.Data[d5] = c.progByte(z)
			if op&1 != 0 {
				c.setWord(30, z+1)
			}
			return 3
		case 0x6, 0x7: // ELPM: no RAMPZ on this chip
			z := c.word(30)
			c.Data[d5] = c.progByte(z)
			if op&1 != 0 {
				c.setWord(30, z+1)
			}
			return 3
		case 0x9, 0xA: // LD Y+, -Y
			c.Data[d5] = c.loadPtr(28, op&0x0F == 9, op&0x0F == 0xA)
			return 2
		case 0xC, 0xD, 0xE: // LD X, X+, -X
			c.Data[d5] = c.loadPtr(26, op&0x0F == 0xD, op&0x0F == 0xE)
			return 2
		case 0xF: // POP
			c.Data[d5] = c.pop()
			return 2
		}
	case op&0xFE00 == 0x9200: // stores
		v := c.Data[d5]
		switch op & 0x0F {
		case 0x0: // STS
			c.Write(int(c.fetch()), v)
			return 2
		case 0x1, 0x2:
			c.storePtr(30, v, op&0x0F == 1, op&0x0F == 2)
			return 2
		case 0x9, 0xA:
			c.storePtr(28, v, op&0x0F == 9, op&0x0F == 0xA)
			return 2
		case 0xC, 0xD, 0xE:
			c.storePtr(26, v, op&0x0F == 0xD, op&0x0F == 0xE)
			return 2
		case 0xF: // PUSH
			c.push(v)
			return 2
		}
	case op&0xFE0E == 0x940C: // JMP
		k := uint32(op>>3&0x3E|op&1)<<16 | uint32(c.fetch())
		c.PC = k
		return 3
	case op&0xFE0E == 0x940E: // CALL
		k := uint32(op>>3&0x3E|op&1)<<16 | uint32(c.fetch())
		c.pushPC(c.PC)
		c.PC = k
		return 4
	case op&0xFF8F == 0x9408: // BSET
		if op>>4&7 == 7 && !c.flag(flagI) {
			c.irqHold = true // SEI: the next instruction first
			c.irqDirty = true
		}
		c.Data[regSREG] |= 1 << (op >> 4 & 7)
		return 1
	case op&0xFF8F == 0x9488: // BCLR
		c.Data[regSREG] &^= 1 << (op >> 4 & 7)
		return 1
	case op == 0x9508: // RET
		c.PC = c.popPC()
		return 4
	case op == 0x9518: // RETI
		c.PC = c.popPC()
		c.setFlag(flagI, true)
		c.irqHold = true
		c.irqDirty = true
		return 4
	case op == 0x9588: // SLEEP
		c.Sleeping = true
		return 1
	case op == 0x9598: // BREAK
		c.Break = true
		return 1
	case op == 0x95A8: // WDR
		return 1
	case op == 0x95C8, op == 0x95D8: // LPM (R0), ELPM
		c.Data[0] = c.progByte(c.word(30))
		return 3
	case op == 0x95E8: // SPM: not emulated
		return 1
	case op == 0x9409, op == 0x9419: // IJMP, EIJMP
		c.PC = uint32(c.word(30))
		return 2
	case op == 0x9509, op == 0x9519: // ICALL, EICALL
		c.pushPC(c.PC)
		c.PC = uint32(c.word(30))
		return 3
	case op&0xFE00 == 0x9400: // one-register operations
		return c.exec1(op, d5)
	case op&0xFE00 == 0x9600: // ADIW / SBIW
		d := 24 + int(op>>4&3)*2
		k := int(op&0x0F | op>>2&0x30)
		v := c.word(d)
		hi := byte(v >> 8)
		var res int
		if op&0x0100 == 0 {
			res = (v + k) & 0xFFFF
			c.setFlag(flagV, hi&0x80 == 0 && res&0x8000 != 0)
			c.setFlag(flagC, res&0x8000 == 0 && hi&0x80 != 0)
		} else {
			res = (v - k) & 0xFFFF
			c.setFlag(flagV, hi&0x80 != 0 && res&0x8000 == 0)
			c.setFlag(flagC, res&0x8000 != 0 && hi&0x80 == 0)
		}
		c.setWord(d, res)
		c.setFlag(flagN, res&0x8000 != 0)
		c.setFlag(flagZ, res == 0)
		c.setFlag(flagS, c.flag(flagN) != c.flag(flagV))
		return 2
	case op&0xFC00 == 0x9800: // CBI, SBIC, SBI, SBIS
		a := ioBase + int(op>>3&0x1F)
		bit := byte(1) << (op & 7)
		switch op & 0x0300 {
		case 0x0000: // CBI
			c.Write(a, c.Read(a)&^bit)
			return 2
		case 0x0100: // SBIC
			if c.Read(a)&bit == 0 {
				return 1 + c.skip()
			}
			return 1
		case 0x0200: // SBI
			c.Write(a, c.Read(a)|bit)
			return 2
		default: // SBIS
			if c.Read(a)&bit != 0 {
				return 1 + c.skip()
			}
			return 1
		}
	case op&0xFC00 == 0x9C00: // MUL
		c.mulResult(int(c.Data[d5])*int(c.Data[r5]), false)
		return 2
	}
	return 1
}

func (c *CPU) exec1(op uint16, d int) int {
	v := c.Data[d]
	switch op & 0x0F {
	case 0x0: // COM
		r := ^v
		c.Data[d] = r
		c.setFlag(flagC, true)
		c.flagsLogic(r)
	case 0x1: // NEG
		r := -v
		c.Data[d] = r
		c.setFlag(flagH, (r|v)&0x08 != 0)
		c.setFlag(flagV, r == 0x80)
		c.setFlag(flagC, r != 0)
		c.setZNS(r)
	case 0x2: // SWAP
		c.Data[d] = v<<4 | v>>4
	case 0x3: // INC
		r := v + 1
		c.Data[d] = r
		c.setFlag(flagV, r == 0x80)
		c.setZNS(r)
	case 0x5: // ASR
		r := v>>1 | v&0x80
		c.Data[d] = r
		c.shiftFlags(v, r)
	case 0x6: // LSR
		r := v >> 1
		c.Data[d] = r
		c.shiftFlags(v, r)
	case 0x7: // ROR
		r := v>>1 | b2i(c.flag(flagC))<<7
		c.Data[d] = r
		c.shiftFlags(v, r)
	case 0xA: // DEC
		r := v - 1
		c.Data[d] = r
		c.setFlag(flagV, v == 0x80)
		c.setZNS(r)
	}
	return 1
}

func (c *CPU) shiftFlags(v, r byte) {
	c.setFlag(flagC, v&1 != 0)
	c.setFlag(flagN, r&0x80 != 0)
	c.setFlag(flagV, c.flag(flagN) != c.flag(flagC))
	c.setFlag(flagS, c.flag(flagN) != c.flag(flagV))
	c.setFlag(flagZ, r == 0)
}

func (c *CPU) progByte(addr int) byte {
	w := c.Prog[(addr>>1)&0x3FFF]
	if addr&1 != 0 {
		return byte(w >> 8)
	}
	return byte(w)
}

func (c *CPU) loadPtr(reg int, post, pre bool) byte {
	a := c.word(reg)
	if pre {
		a = (a - 1) & 0xFFFF
		c.setWord(reg, a)
	}
	v := c.Read(a)
	if post {
		c.setWord(reg, a+1)
	}
	return v
}

func (c *CPU) storePtr(reg int, v byte, post, pre bool) {
	a := c.word(reg)
	if pre {
		a = (a - 1) & 0xFFFF
		c.setWord(reg, a)
	}
	c.Write(a, v)
	if post {
		c.setWord(reg, a+1)
	}
}
