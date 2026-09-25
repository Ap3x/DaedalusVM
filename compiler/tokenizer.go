package main

import (
	"fmt"
	"strings"
)

type TokenType string

const (
	TokInt         TokenType = "INT"
	TokString      TokenType = "STRING"
	TokLabel       TokenType = "LABEL"
	TokAddress     TokenType = "ADDRESS"
	TokInstruction TokenType = "INSTRUCTION"
	TokRegister    TokenType = "REGISTER"
)

type Token struct {
	Type  TokenType
	Value string
	Res   []byte
}

var Instructions = []string{"add", "sub", "mul", "div", "mod", "jmp", "jz", "jnz", "cmp", "ret", "push", "pop", "load", "store", "go2", "mov", "halt"}
var Registers = []string{"ax", "bx", "cx", "dx", "sp", "bp", "si", "di"}

// JumpInstructions take a label operand (an address) as their next token.
var JumpInstructions = map[string]bool{"jmp": true, "jz": true, "jnz": true, "go2": true}
var RegisterSizes = []string{"r", "e", "u", "l"} // r = 64-bit, e = 32-bit, u = 16-bit, l = 8-bit

func checkInstruction(line []rune) (Token, error) {
	for _, instruction := range Instructions {
		if strings.HasPrefix(string(line), instruction) {
			return Token{Type: TokInstruction, Value: instruction}, nil
		}
	}
	return Token{}, fmt.Errorf("unexpected instruction: %q", string(line))
}

func checkRegisterSizes(line []rune) (Token, error) {
	for _, reg := range RegisterSizes {
		if strings.HasPrefix(string(line), reg) {
			return Token{Type: TokRegister, Value: reg}, nil
		}
	}
	return Token{}, fmt.Errorf("unknown register size: %q", string(line))
}

func checkRegister(line []rune) (Token, error) {
	_, err := checkRegisterSizes(line)
	if err == nil {
		for _, reg := range Registers {
			if strings.HasPrefix(string(line[1:]), reg) {
				return Token{Type: TokRegister, Value: string(line)}, nil
			}
		}
	}

	return Token{}, fmt.Errorf("register error: %q", err)
}

func tokenizeLine(lineNum int, line string) ([]Token, error) {
	var tokens []Token
	line = strings.Split(line, "//")[0] // remove comments
	line = strings.TrimSpace(line)
	line = strings.ToLower(line)
	rs := []rune(line) // slice of runes

	if len(rs) == 0 {
		return tokens, nil
	}

	cursor := 0
	column := 1

	for cursor < len(rs) {
		if cursor == len(rs) {
			break
		}

		switch {
		// Whitespace
		case rs[cursor] == ' ':
			cursor++
		// Instructions
		case rs[cursor] >= 'a' && rs[cursor] <= 'z':
			token, err := checkInstruction(rs[cursor:])
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			cursor += len(token.Value)
			column++
		// Labels
		case rs[cursor] == '.':
			labelType := TokAddress
			if len(tokens) == 0 {
				labelType = TokLabel
			}
			tokens = append(tokens, Token{Type: labelType, Value: string(rs[cursor+1:])})
			cursor += len(rs) - cursor
		// Registers
		case rs[cursor] == '@':
			registerSnip := rs[cursor+1 : cursor+4]
			token, err := checkRegister(registerSnip)
			if err != nil {
				return nil, err
			}

			tokens = append(tokens, token)
			cursor += len(token.Value) + 1
			column++
		// Integer literals
		case rs[cursor] >= '0' && rs[cursor] <= '9':
			extractedNumber := strings.Split(string(rs[cursor:]), ",")[0]
			extractedNumber = strings.TrimSpace(extractedNumber)
			tokens = append(tokens, Token{
				Type:  TokInt,
				Value: extractedNumber,
			})
			cursor += len(extractedNumber)
			column++
		// String literals
		case rs[cursor] == '"':
			tokenSnip := rs[cursor+1 : len(rs)-1]
			tokens = append(tokens, Token{
				Type:  TokString,
				Value: string(tokenSnip),
			})
			cursor += len(string(tokenSnip)) + 2
		case rs[cursor] == ',':
			cursor++
		default:
			return nil, fmt.Errorf("unexpected token: %q", string(rs))
		}
	}

	return tokens, nil
}
