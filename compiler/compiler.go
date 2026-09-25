package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: compiler <source-file>")
		os.Exit(1)
	}

	if err := compile(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func compile(path string) error {
	var labels = make(map[string]uint16)

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB
	toks := []Token{}
	lineNum := 0
	for scanner.Scan() {
		lineNum++

		lineToks, err := tokenizeLine(lineNum, scanner.Text())
		if err != nil {
			return fmt.Errorf("%s:%w", path, err)
		}

		toks = append(toks, lineToks...)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Reject duplicate label definitions before resolving anything.
	seen := make(map[string]bool)
	for i := range toks {
		if toks[i].Type == TokLabel && toks[i-1].Type != TokInstruction {
			if seen[toks[i].Value] {
				return fmt.Errorf("%s: duplicate label %q", path, toks[i].Value)
			}
			seen[toks[i].Value] = true
		}
	}

	// Jump-family instructions must be followed by a label operand.
	for i := range toks {
		if toks[i].Type != TokInstruction || !JumpInstructions[toks[i].Value] {
			continue
		}
		if i+1 >= len(toks) || toks[i+1].Type != TokAddress {
			return fmt.Errorf("%s: %q requires a label operand", path, toks[i].Value)
		}
	}

	// Pass 1: fill in each label's byte offset. A forward reference emits a
	// placeholder here, but its width (2 bytes) matches pass 2, so offsets hold.
	var scratch []byte
	for i := range toks {
		if err := token2Bytes(i, toks, &scratch, labels); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	// Every reference must point at a label that was actually defined.
	for i := range toks {
		if toks[i].Type == TokAddress {
			if _, ok := labels[toks[i].Value]; !ok {
				return fmt.Errorf("%s: undefined label %q", path, toks[i].Value)
			}
		}
	}

	// Pass 2: emit the real bytecode now that all addresses are known.
	var bufToks []byte
	for i := range toks {
		if err := token2Bytes(i, toks, &bufToks, labels); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	print_c_code(bufToks)

	// Also emit a raw binary next to the source (foo.vm -> foo.bin) so the
	// loader can execute it directly: `Ap3xVMLoader foo.bin`.
	binPath := strings.TrimSuffix(path, ".vm") + ".bin"
	if err := os.WriteFile(binPath, bufToks, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", binPath, err)
	}

	return nil
}

func print_c_code(bufToks []byte) {
	// C array initializer, ready to paste into a C program.
	fmt.Printf("unsigned char bytecode[%d] = {", len(bufToks))
	for i, b := range bufToks {
		if i > 0 {
			fmt.Printf(", ")
		}
		fmt.Printf("0x%02X", b)
	}
	fmt.Printf("};\n")
}
