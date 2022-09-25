package Type

type Type int

func (t Type) String() string {
	switch t {
	case Invalid:
		return "invalid"
	case I8:
		return "i8"
	case I16:
		return "i16"
	case I32:
		return "i32"
	case I64:
		return "i64"
	case Bool:
		return "bool"
	case Ptr:
		return "ptr"
	case Syscall:
		return "syscall"
	case Proc:
		return "Proc"
	case MultiRet:
		return "MultiRet"
	}
	panic("Type.String(): Invalid Type")
}

const (
	Invalid Type = iota

	I8
	I16
	I32
	I64
	Bool
	Ptr

	MultiRet
	Proc
	Syscall
)
