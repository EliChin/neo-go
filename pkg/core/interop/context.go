package interop

import (
	"github.com/nspcc-dev/neo-go/pkg/core/block"
	"github.com/nspcc-dev/neo-go/pkg/core/blockchainer"
	"github.com/nspcc-dev/neo-go/pkg/core/dao"
	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto"
	"github.com/nspcc-dev/neo-go/pkg/crypto/hash"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm"
	"github.com/nspcc-dev/neo-go/pkg/vm/emit"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"go.uber.org/zap"
)

// Context represents context in which interops are executed.
type Context struct {
	Chain         blockchainer.Blockchainer
	Container     crypto.Verifiable
	Natives       []Contract
	Trigger       trigger.Type
	Block         *block.Block
	Tx            *transaction.Transaction
	DAO           *dao.Cached
	Notifications []state.NotificationEvent
	Log           *zap.Logger
	Invocations   map[util.Uint160]int
	ScriptGetter  vm.ScriptHashGetter
}

// NewContext returns new interop context.
func NewContext(trigger trigger.Type, bc blockchainer.Blockchainer, d dao.DAO, natives []Contract, block *block.Block, tx *transaction.Transaction, log *zap.Logger) *Context {
	dao := dao.NewCached(d)
	nes := make([]state.NotificationEvent, 0)
	return &Context{
		Chain:         bc,
		Natives:       natives,
		Trigger:       trigger,
		Block:         block,
		Tx:            tx,
		DAO:           dao,
		Notifications: nes,
		Log:           log,
		Invocations:   make(map[util.Uint160]int),
	}
}

// Function binds function name, id with the function itself and price,
// it's supposed to be inited once for all interopContexts, so it doesn't use
// vm.InteropFuncPrice directly.
type Function struct {
	ID    uint32
	Name  string
	Func  func(*Context, *vm.VM) error
	Price int64
	// RequiredFlags is a set of flags which must be set during script invocations.
	// Default value is NoneFlag i.e. no flags are required.
	RequiredFlags smartcontract.CallFlag
}

// Method is a signature for a native method.
type Method = func(ic *Context, args []stackitem.Item) stackitem.Item

// MethodAndPrice is a native-contract method descriptor.
type MethodAndPrice struct {
	Func          Method
	Price         int64
	RequiredFlags smartcontract.CallFlag
}

// Contract is an interface for all native contracts.
type Contract interface {
	Initialize(*Context) error
	Metadata() *ContractMD
}

// ContractMD represents native contract instance.
type ContractMD struct {
	Manifest   manifest.Manifest
	Name       string
	ContractID int32
	Script     []byte
	Hash       util.Uint160
	Methods    map[string]MethodAndPrice
}

// NewContractMD returns Contract with the specified list of methods.
func NewContractMD(name string) *ContractMD {
	c := &ContractMD{
		Name:    name,
		Methods: make(map[string]MethodAndPrice),
	}

	w := io.NewBufBinWriter()
	emit.String(w.BinWriter, c.Name)
	emit.Syscall(w.BinWriter, "Neo.Native.Call")

	c.Script = w.Bytes()
	c.Hash = hash.Hash160(c.Script)
	c.Manifest = *manifest.DefaultManifest(c.Hash)

	return c
}

// AddMethod adds new method to a native contract.
func (c *ContractMD) AddMethod(md *MethodAndPrice, desc *manifest.Method, safe bool) {
	c.Manifest.ABI.Methods = append(c.Manifest.ABI.Methods, *desc)
	c.Methods[desc.Name] = *md
	if safe {
		c.Manifest.SafeMethods.Add(desc.Name)
	}
}

// AddEvent adds new event to a native contract.
func (c *ContractMD) AddEvent(name string, ps ...manifest.Parameter) {
	c.Manifest.ABI.Events = append(c.Manifest.ABI.Events, manifest.Event{
		Name:       name,
		Parameters: ps,
	})
}
