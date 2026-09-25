#include <windows.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

#define NUM_REGS 8
#define STACK_SIZE 10*1024*1024 // 10MB
#define REG_SP 7                // Register used for stack

typedef enum {
    OP_ADD,
    OP_SUB,
    OP_MUL,
    OP_DIV,
    OP_MOD,
    OP_JMP,
    OP_JZ,
    OP_JNZ,
    OP_CMP,
    OP_RET,
    OP_PUSH,
    OP_POP,
    OP_LOAD,
    OP_STORE,
    OP_GOTO,
    OP_MOV,
    OP_HALT
} opcode_t;

typedef struct {
    opcode_t   op;            /* opcode        */
    uint8_t   op_pre;       /* mode          */
    uint8_t   store_reg;       /* mode          */
    uint64_t  a;            /* operand A     */
    uint64_t  b;            /* operand B     */
    char*     s;            /* string        */
} Instr;

typedef struct {
    uint64_t            regs[NUM_REGS];
    uint64_t            stack[STACK_SIZE];
    uint8_t             zero_flag;
    uint8_t             running;
    size_t              n_instr;        /* instruction count */
    uint16_t            pc;             /* program counter (addresses are 16-bit) */
    unsigned char* prog;                  /* bytecode buffer   */
} VM;

/* Read `width` little-endian bytes at prog[at] into a uint64_t. */
static uint64_t read_imm(const unsigned char* prog, size_t at, int width) {
    uint64_t v = 0;
    for (int k = 0; k < width; k++)
        v |= (uint64_t)prog[at + k] << (8 * k);
    return v;
}

/* Width in bytes of a register, from its size nibble (high nibble of the
   register byte): 0=r=8, 1=e=4, 2=u=2, 3=l=1. Encoding is size<<4 | index. */
static int reg_width(uint8_t regByte) {
    switch (regByte >> 4) {
    case 0: return 8; /* r: 64-bit */
    case 1: return 4; /* e: 32-bit */
    case 2: return 2; /* u: 16-bit */
    case 3: return 1; /* l: 8-bit  */
    }
    return 0;
}

/* Read a register's value narrowed to its declared size, so `lax` yields the
   low 8 bits of ax, `eax` the low 32, etc. Encoding is size<<4 | index. */
static uint64_t reg_value(const VM* vm, uint8_t regByte) {
    int w = reg_width(regByte);
    uint64_t v = vm->regs[regByte & 0x0F];
    if (w >= 8) return v;
    return v & (((uint64_t)1 << (8 * w)) - 1);
}

/* Operand convention (set by parse_instruction, honored by every handler):
   store_reg = destination register index; a = value of operand 1;
   b = value of operand 2 (register value or immediate). */
static void h_mov(VM* vm, Instr instruction) {
    vm->regs[instruction.store_reg] = instruction.b;
}

static void h_add(VM* vm, Instr instruction) {
    vm->regs[instruction.store_reg] = instruction.a + instruction.b;
}

static void h_sub(VM* vm, Instr instruction) {
    vm->regs[instruction.store_reg] = instruction.a - instruction.b;
}

static void h_multiply(VM* vm, Instr instruction) {
    vm->regs[instruction.store_reg] = instruction.a * instruction.b;
}

static void h_divide(VM* vm, Instr instruction) {
    if (instruction.b == 0) { vm->running = 0; return; } /* no div-by-zero trap */
    vm->regs[instruction.store_reg] = instruction.a / instruction.b;
}

static void h_mod(VM* vm, Instr instruction) {
    if (instruction.b == 0) { vm->running = 0; return; }
    vm->regs[instruction.store_reg] = instruction.a % instruction.b;
}

/* jmp <label>: set the program counter to the target address.
   instruction.a is the absolute byte offset of the target (from prefix 4).
   The exec loop does ++vm->pc after each handler, so we set pc to target-1
   and let that increment land exactly on the target. */
static void h_jump(VM* vm, Instr instruction) {
    vm->pc = (uint16_t)(instruction.a - 1);
}

static void h_jump_zero(VM* vm, Instr instruction) {
    if (vm->zero_flag == 1){
        vm->pc = (uint16_t)(instruction.a - 1);
    }
}

static void h_jump_not_zero(VM* vm, Instr instruction) {
    if (vm->zero_flag != 1) {
        vm->pc = (uint16_t)(instruction.a - 1);
    }
}

static void h_compare(VM* vm, Instr instruction) {
    vm->zero_flag = (instruction.a == instruction.b) ? 1 : 0;
}

static void h_return(VM* vm, Instr instruction) {

}

static void h_push(VM* vm, Instr instruction) {
    if (vm->regs[REG_SP] >= STACK_SIZE) { 
        vm->running = 0;
        return;
    }
    vm->stack[vm->regs[REG_SP]++] = instruction.a;
}

static void h_pop(VM* vm, Instr instruction) {
    if (vm->regs[REG_SP] == 0) {         
        vm->running = 0;
        return;
    }
    vm->regs[instruction.store_reg] = vm->stack[--vm->regs[REG_SP]];
}

static void h_load(VM* vm, Instr instruction) {

}

static void h_store(VM* vm, Instr instruction) {
    for (uint64_t i = 0; i < instruction.a; i++) {
        if (vm->regs[REG_SP] >= STACK_SIZE) { /* stack full -> stop the VM */
            vm->running = 0;
            return;
        }
        vm->stack[vm->regs[REG_SP]++] = (uint8_t)instruction.s[i];
    }
}

static void h_goto(VM* vm, Instr instruction) {
    vm->pc = (uint16_t)(instruction.a - 1);
}

/* Opcode byte layout: prefix in the high 3 bits, instruction index in the low
   5 bits. 5 bits is required because there are 17 instructions (add..halt =
   0..16) and index 16 does not fit in a nibble. Prefixes only range 0..5. */
opcode_t get_opcode(uint8_t b) {
    opcode_t opcode = (opcode_t)(b & 0x1F);   /* low 5 bits = instruction index */
    return opcode;
}

uint8_t get_op_prefix(uint8_t b) {
    uint8_t prefix = b >> 5;                  /* high 3 bits = operand prefix */
    return prefix;
}

Instr parse_instruction(VM* vm) {
    Instr instruction = {};   /* zero-init: prefixes that skip a field leave it 0, not garbage */
    instruction.op = get_opcode((uint8_t)vm->prog[vm->pc]);
    instruction.op_pre = get_op_prefix((uint8_t)vm->prog[vm->pc]);

    switch (instruction.op_pre) {
    case 0: { /* reg, reg -- operands are the size-masked register values */
        uint8_t reg1Byte = vm->prog[vm->pc + 1];
        uint8_t reg2Byte = vm->prog[vm->pc + 2];
        instruction.store_reg = reg1Byte & 0x0F;         /* dest register INDEX */
        instruction.a = reg_value(vm, reg1Byte);         /* value of operand 1  */
        instruction.b = reg_value(vm, reg2Byte);         /* value of operand 2  */
        vm->pc += 2;
        break;
    }
    case 1: { /* reg, immediate (immediate width = register size) */
        uint8_t reg1Byte = vm->prog[vm->pc + 1];
        int width = reg_width(reg1Byte);
        instruction.store_reg = reg1Byte & 0x0F;         /* dest register INDEX */
        instruction.a = reg_value(vm, reg1Byte);         /* current value of dest */
        instruction.b = read_imm(vm->prog, vm->pc + 2, width);
        vm->pc += 1 + width;                             /* reg byte + immediate */
        break;
    }
    case 2: { /* immediate, immediate -- compiler emits two 8-byte little-endian values */
        instruction.a = read_imm(vm->prog, vm->pc + 1, 8);
        instruction.b = read_imm(vm->prog, vm->pc + 9, 8);
        vm->pc += 16;
        break;
    }
    case 3: { /* string: [length][chars...] */
        uint8_t length = vm->prog[vm->pc + 1];
        instruction.a = length;                          /* keep the length in a */
        instruction.s = (char*)&vm->prog[vm->pc + 2];    /* points into prog, not null-terminated */
        vm->pc += 1 + length;                            /* length byte + chars */
        break;
    }
    case 4: { /* label: 2-byte little-endian address */
        instruction.a = (uint16_t)(vm->prog[vm->pc + 1] | (vm->prog[vm->pc + 2] << 8));
        vm->pc += 2;                                     /* two address bytes */
        break;
    }
    case 5: /* no operands (ret, halt) -- loop's ++pc covers the opcode byte */
        break;
    case 6: { /* single register (push, pop) */
        uint8_t regByte = vm->prog[vm->pc + 1];
        instruction.store_reg = regByte & 0x0F;      /* register INDEX  (pop's dest) */
        instruction.a = reg_value(vm, regByte);      /* register VALUE  (push's src) */
        vm->pc += 1;
        break;
    }
    }

    return instruction;
}

static void vm_exec(VM* vm) {
    for (vm->pc = 0; vm->running && vm->pc < vm->n_instr; ++vm->pc) {
        Instr instruction = parse_instruction(vm);

        switch (instruction.op) {
        case OP_ADD:          h_add(vm,instruction); break;
        case OP_SUB:          h_sub(vm,instruction); break;
        case OP_MUL:          h_multiply(vm,instruction); break;
        case OP_DIV:          h_divide(vm,instruction); break;
        case OP_MOD:          h_mod(vm,instruction); break;
        case OP_JMP:          h_jump(vm,instruction); break;
        case OP_JZ:          h_jump_zero(vm,instruction); break;
        case OP_JNZ:          h_jump_not_zero(vm,instruction); break;
        case OP_CMP:          h_compare(vm,instruction); break;
        case OP_RET:          h_return(vm,instruction); break;
        case OP_PUSH:          h_push(vm,instruction); break;
        case OP_POP:          h_pop(vm,instruction); break;
        case OP_LOAD:          h_load(vm,instruction); break;
        case OP_STORE:          h_store(vm,instruction); break;
        case OP_GOTO:          h_goto(vm,instruction); break;
        case OP_MOV:          h_mov(vm,instruction); break;
        case OP_HALT:         vm->running = 0; break;
        default:              break; 
        }
    }
}

/* Read an entire bytecode file into a freshly malloc'd buffer. */
static unsigned char* load_file(const char* path, size_t* out_len) {
    FILE* f = fopen(path, "rb");
    if (!f) return NULL;
    fseek(f, 0, SEEK_END);
    long sz = ftell(f);
    fseek(f, 0, SEEK_SET);
    if (sz <= 0) { fclose(f); return NULL; }
    unsigned char* buf = (unsigned char*)malloc((size_t)sz);
    if (!buf) { fclose(f); return NULL; }
    *out_len = fread(buf, 1, (size_t)sz, f);
    fclose(f);
    return buf;
}

int main(int argc, char** argv) {
    VM* vm = (VM*)calloc(1, sizeof(VM));
    if (!vm) return 1;

    unsigned char builtin[35] = { 0x00 };

    unsigned char* loaded = NULL;
    if (argc >= 2) {
        size_t len = 0;
        loaded = load_file(argv[1], &len);
        if (!loaded) {
            fprintf(stderr, "error: could not read bytecode file %s\n", argv[1]);
            free(vm);
            return 1;
        }
        vm->prog = loaded;
        vm->n_instr = len;
    } else {
        vm->prog = builtin;
        vm->n_instr = sizeof(builtin) / sizeof(builtin[0]);
    }
    vm->running = (uint8_t)1;

    vm_exec(vm);

    for (int i = 0; i < NUM_REGS; i++)
        printf("regs[%d] = %llu\n", i, (unsigned long long)vm->regs[i]);

    printf("stack (sp=%llu): ", (unsigned long long)vm->regs[REG_SP]);
    for (uint64_t i = 0; i < vm->regs[REG_SP] && i < 32; i++)
        printf("%c", (char)vm->stack[i]);
    printf("%s\n", vm->regs[REG_SP] > 32 ? " ..." : "");

    free(loaded);
    free(vm);
    return 0;
}
