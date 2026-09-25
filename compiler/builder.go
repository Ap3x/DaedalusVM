package main

import (
	"fmt"
	"strconv"
	"strings"
)

type RegIndex int
type RegSizeIndex int
type InstructionIndex int
type InstructionPrefix int

const (
	ax RegIndex = iota
	bx
	cx
	dx
	sp
	bp
	si
	di
)

const (
	r RegSizeIndex = iota
	e
	u
	l
)

const (
	add InstructionIndex = iota
	sub
	mul
	div
	mod
	jmp
	jz
	jnz
	cmp
	ret
	push
	pop
	load
	store
	gotoLabel
	mov
	halt
)

func token2Bytes(i int, toks []Token, buf *[]byte, labels map[string]uint16) error {
	t := &toks[i]

	switch t.Type {
	case TokRegister:
		// One byte: high nibble = size (r/e/u/l), low nibble = index (ax..di).
		// e.g. lax -> 0x30, rbx -> 0x01, edi -> 0x17.
		var size RegSizeIndex
		switch t.Value[0] {
		case 'r':
			size = r
		case 'e':
			size = e
		case 'u':
			size = u
		case 'l':
			size = l
		}

		var index RegIndex
		switch t.Value[1:] {
		case "ax":
			index = ax
		case "bx":
			index = bx
		case "cx":
			index = cx
		case "dx":
			index = dx
		case "sp":
			index = sp
		case "bp":
			index = bp
		case "si":
			index = si
		case "di":
			index = di
		}

		regByte := byte(size)<<4 | byte(index)
		t.Res = []byte{regByte}
		*buf = append(*buf, regByte)
	case TokInt:
		var n uint64
		var err error

		if strings.HasPrefix(t.Value, "0x") {
			n, err = strconv.ParseUint(t.Value, 0, 64) // Hexadecimal
		} else {
			n, err = strconv.ParseUint(t.Value, 10, 64) // Decimal
		}
		if err != nil {
			return fmt.Errorf("invalid integer %q: %w", t.Value, err)
		}

		// The immediate width follows the destination register (the previous
		// token), so the VM reads exactly this many bytes. Default to 8.
		width := 8
		if i > 0 && toks[i-1].Type == TokRegister {
			switch toks[i-1].Value[0] {
			case 'r':
				width = 8
			case 'e':
				width = 4
			case 'u':
				width = 2
			case 'l':
				width = 1
			}
		}

		if width < 8 && n >= (uint64(1)<<(8*width)) {
			return fmt.Errorf("value %s does not fit in a %d-byte register", t.Value, width)
		}

		intBytes := make([]byte, width)
		for k := 0; k < width; k++ {
			intBytes[k] = byte(n >> (8 * k)) // little-endian
		}

		t.Res = intBytes
		*buf = append(*buf, intBytes...)
	case TokString:
		var calculatedBytes []byte
		t.Value = strings.ReplaceAll(t.Value, "\"", "") // just spaces
		length := len(t.Value)
		calculatedBytes = append(calculatedBytes, byte(length))
		characters := []rune(t.Value)
		for _, char := range characters {
			calculatedBytes = append(calculatedBytes, byte(char))
		}
		t.Res = calculatedBytes
		*buf = append(*buf, calculatedBytes...)
	case TokInstruction:
		// Gather the operand token types that follow this instruction (up to
		// two), stopping at the next instruction.
		var combo []TokenType
		for j := i + 1; j < len(toks) && len(combo) < 2; j++ {
			if toks[j].Type == TokInstruction || toks[j].Type == TokLabel {
				break // next instruction or a label definition ends the operands
			}
			combo = append(combo, toks[j].Type)
		}

		var prefix int
		switch {
		case len(combo) == 0:
			prefix = 5 // no operands (e.g. ret, halt)
		case len(combo) == 2 && combo[0] == TokRegister && combo[1] == TokRegister:
			prefix = 0
		case len(combo) == 2 && combo[0] == TokRegister && combo[1] == TokInt:
			prefix = 1
		case len(combo) == 2 && combo[0] == TokInt && combo[1] == TokInt:
			prefix = 2
		case combo[0] == TokString:
			prefix = 3
		case combo[0] == TokAddress:
			prefix = 4
		default:
			return fmt.Errorf("unrecognized operand combination after %q: %v", t.Value, combo)
		}

		var instructionIndex InstructionIndex
		switch t.Value {
		case "add":
			instructionIndex = add
		case "sub":
			instructionIndex = sub
		case "mul":
			instructionIndex = mul
		case "div":
			instructionIndex = div
		case "mod":
			instructionIndex = mod
		case "jmp":
			instructionIndex = jmp
		case "jz":
			instructionIndex = jz
		case "jnz":
			instructionIndex = jnz
		case "cmp":
			instructionIndex = cmp
		case "ret":
			instructionIndex = ret
		case "push":
			instructionIndex = push
		case "pop":
			instructionIndex = pop
		case "load":
			instructionIndex = load
		case "store":
			instructionIndex = store
		case "go2":
			instructionIndex = gotoLabel
		case "mov":
			instructionIndex = mov
		case "halt":
			instructionIndex = halt
		}
		// Opcode byte: prefix in the high 3 bits, instruction index in the low
		// 5 bits. 5 bits is needed because halt is index 16, which overflows a
		// nibble; prefixes only range 0..5 so 3 bits is plenty.
		regByte := byte(prefix)<<5 | byte(instructionIndex)
		t.Res = []byte{regByte}
		*buf = append(*buf, regByte)
	case TokLabel:
		// Definition: record this position; the label itself emits no bytes.
		if _, exists := labels[t.Value]; !exists {
			labels[t.Value] = uint16(len(*buf))
		}
	case TokAddress:
		// Reference: emit the resolved address (validated before this pass).
		addr := labels[t.Value]
		t.Res = []byte{byte(addr), byte(addr >> 8)} // 2-byte little-endian
		*buf = append(*buf, t.Res...)
	default:
		fmt.Printf("Unknown token type: %s\n", t.Type)
	}

	return nil
}
