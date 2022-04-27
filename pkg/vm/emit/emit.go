package emit

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"math/bits"

	"github.com/nspcc-dev/neo-go/pkg/core/interop/interopnames"
	"github.com/nspcc-dev/neo-go/pkg/encoding/bigint"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/callflag"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/opcode"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

// Instruction emits a VM Instruction with data to the given buffer.
func Instruction(w *io.BinWriter, op opcode.Opcode, b []byte) {
	w.WriteB(byte(op))
	w.WriteBytes(b)
}

// Opcodes emits a single VM Instruction without arguments to the given buffer.
func Opcodes(w *io.BinWriter, ops ...opcode.Opcode) {
	for _, op := range ops {
		w.WriteB(byte(op))
	}
}

// Bool emits a bool type to the given buffer.
func Bool(w *io.BinWriter, ok bool) {
	if ok {
		Opcodes(w, opcode.PUSHT)
	} else {
		Opcodes(w, opcode.PUSHF)
	}
	Instruction(w, opcode.CONVERT, []byte{byte(stackitem.BooleanT)})
}

func padRight(s int, buf []byte) []byte {
	l := len(buf)
	buf = buf[:s]
	if buf[l-1]&0x80 != 0 {
		for i := l; i < s; i++ {
			buf[i] = 0xFF
		}
	}
	return buf
}

// Int emits an int type to the given buffer.
func Int(w *io.BinWriter, i int64) {
	if smallInt(w, i) {
		return
	}
	bigInt(w, big.NewInt(i), false)
}

// BigInt emits a big-integer to the given buffer.
func BigInt(w *io.BinWriter, n *big.Int) {
	bigInt(w, n, true)
}

func smallInt(w *io.BinWriter, i int64) bool {
	switch {
	case i == -1:
		Opcodes(w, opcode.PUSHM1)
	case i >= 0 && i < 16:
		val := opcode.Opcode(int(opcode.PUSH0) + int(i))
		Opcodes(w, val)
	default:
		return false
	}
	return true
}

func bigInt(w *io.BinWriter, n *big.Int, trySmall bool) {
	if w.Err != nil {
		return
	}
	if trySmall && n.IsInt64() && smallInt(w, n.Int64()) {
		return
	}

	if err := stackitem.CheckIntegerSize(n); err != nil {
		w.Err = err
		return
	}

	buf := bigint.ToPreallocatedBytes(n, make([]byte, 0, 32))
	if len(buf) == 0 {
		Opcodes(w, opcode.PUSH0)
		return
	}
	padSize := byte(8 - bits.LeadingZeros8(byte(len(buf)-1)))
	Opcodes(w, opcode.PUSHINT8+opcode.Opcode(padSize))
	w.WriteBytes(padRight(1<<padSize, buf))
}

// Array emits an array of elements to the given buffer.
func Array(w *io.BinWriter, es ...interface{}) {
	if len(es) == 0 {
		Opcodes(w, opcode.NEWARRAY0)
		return
	}
	for i := len(es) - 1; i >= 0; i-- {
		switch e := es[i].(type) {
		case []interface{}:
			Array(w, e...)
		case int64:
			Int(w, e)
		case int32:
			Int(w, int64(e))
		case uint32:
			Int(w, int64(e))
		case int16:
			Int(w, int64(e))
		case uint16:
			Int(w, int64(e))
		case int8:
			Int(w, int64(e))
		case uint8:
			Int(w, int64(e))
		case int:
			Int(w, int64(e))
		case *big.Int:
			BigInt(w, e)
		case string:
			String(w, e)
		case util.Uint160:
			Bytes(w, e.BytesBE())
		case util.Uint256:
			Bytes(w, e.BytesBE())
		case []byte:
			Bytes(w, e)
		case bool:
			Bool(w, e)
		default:
			if es[i] != nil {
				w.Err = fmt.Errorf("unsupported type: %T", e)
				return
			}
			Opcodes(w, opcode.PUSHNULL)
		}
	}
	Int(w, int64(len(es)))
	Opcodes(w, opcode.PACK)
}

// String emits a string to the given buffer.
func String(w *io.BinWriter, s string) {
	Bytes(w, []byte(s))
}

// Bytes emits a byte array to the given buffer.
func Bytes(w *io.BinWriter, b []byte) {
	var n = len(b)

	switch {
	case n < 0x100:
		Instruction(w, opcode.PUSHDATA1, []byte{byte(n)})
	case n < 0x10000:
		buf := make([]byte, 2)
		binary.LittleEndian.PutUint16(buf, uint16(n))
		Instruction(w, opcode.PUSHDATA2, buf)
	default:
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, uint32(n))
		Instruction(w, opcode.PUSHDATA4, buf)
	}
	w.WriteBytes(b)
}

// Syscall emits the syscall API to the given buffer.
// Syscall API string cannot be 0.
func Syscall(w *io.BinWriter, api string) {
	if w.Err != nil {
		return
	} else if len(api) == 0 {
		w.Err = errors.New("syscall api cannot be of length 0")
		return
	}
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, interopnames.ToID([]byte(api)))
	Instruction(w, opcode.SYSCALL, buf)
}

// Call emits a call Instruction with the label to the given buffer.
func Call(w *io.BinWriter, op opcode.Opcode, label uint16) {
	Jmp(w, op, label)
}

// Jmp emits a jump Instruction along with the label to the given buffer.
func Jmp(w *io.BinWriter, op opcode.Opcode, label uint16) {
	if w.Err != nil {
		return
	} else if !isInstructionJmp(op) {
		w.Err = fmt.Errorf("opcode %s is not a jump or call type", op.String())
		return
	}
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint16(buf, label)
	Instruction(w, op, buf)
}

// AppCallNoArgs emits a call to the provided contract.
func AppCallNoArgs(w *io.BinWriter, scriptHash util.Uint160, operation string, f callflag.CallFlag) {
	Int(w, int64(f))
	String(w, operation)
	Bytes(w, scriptHash.BytesBE())
	Syscall(w, interopnames.SystemContractCall)
}

// AppCall emits an APPCALL with the default parameters to the given operation and arguments.
func AppCall(w *io.BinWriter, scriptHash util.Uint160, operation string, f callflag.CallFlag, args ...interface{}) {
	Array(w, args...)
	AppCallNoArgs(w, scriptHash, operation, f)
}

func isInstructionJmp(op opcode.Opcode) bool {
	return opcode.JMP <= op && op <= opcode.CALLL || op == opcode.ENDTRYL
}
