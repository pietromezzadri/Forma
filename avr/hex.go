package avr

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// ParseHex reads an Intel HEX file (what the Arduino IDE's "Export
// compiled binary" and avr-objcopy write) into program words.
func ParseHex(r io.Reader) ([]uint16, error) {
	mem := make([]byte, FlashSize)
	top := 0
	base := 0
	sc := bufio.NewScanner(r)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		if s == "" {
			continue
		}
		if s[0] != ':' {
			return nil, fmt.Errorf("line %d: not a HEX record", line)
		}
		b, err := hex.DecodeString(s[1:])
		if err != nil || len(b) < 5 {
			return nil, fmt.Errorf("line %d: bad record", line)
		}
		n := int(b[0])
		if len(b) != n+5 {
			return nil, fmt.Errorf("line %d: length %d, the record says %d", line, len(b)-5, n)
		}
		sum := byte(0)
		for _, x := range b {
			sum += x
		}
		if sum != 0 {
			return nil, fmt.Errorf("line %d: checksum", line)
		}
		addr := int(b[1])<<8 | int(b[2])
		data := b[4 : 4+n]
		switch b[3] {
		case 0x00:
			a := base + addr
			if a+n > len(mem) {
				return nil, fmt.Errorf("line %d: past the 32 KB flash", line)
			}
			copy(mem[a:], data)
			if a+n > top {
				top = a + n
			}
		case 0x01:
			goto done
		case 0x02:
			if n == 2 {
				base = (int(data[0])<<8 | int(data[1])) << 4
			}
		case 0x04:
			if n == 2 {
				base = (int(data[0])<<8 | int(data[1])) << 16
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
done:
	words := make([]uint16, (top+1)/2)
	for i := range words {
		words[i] = uint16(mem[2*i]) | uint16(mem[2*i+1])<<8
	}
	return words, nil
}
