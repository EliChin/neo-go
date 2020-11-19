package core

import (
	"math/big"
	"sort"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/config/netmode"
	"github.com/nspcc-dev/neo-go/pkg/core/native"
	"github.com/nspcc-dev/neo-go/pkg/core/transaction"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neo-go/pkg/internal/testchain"
	"github.com/nspcc-dev/neo-go/pkg/io"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/trigger"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm"
	"github.com/nspcc-dev/neo-go/pkg/vm/emit"
	"github.com/nspcc-dev/neo-go/pkg/vm/opcode"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
	"github.com/nspcc-dev/neo-go/pkg/wallet"
	"github.com/stretchr/testify/require"
)

func setSigner(tx *transaction.Transaction, h util.Uint160) {
	tx.Signers = []transaction.Signer{{
		Account: h,
		Scopes:  transaction.Global,
	}}
}

func checkTxHalt(t *testing.T, bc *Blockchain, h util.Uint256) {
	aer, err := bc.GetAppExecResults(h, trigger.Application)
	require.NoError(t, err)
	require.Equal(t, 1, len(aer))
	require.Equal(t, vm.HaltState, aer[0].VMState, aer[0].FaultException)
}

func TestNEO_Vote(t *testing.T) {
	bc := newTestChain(t)
	defer bc.Close()

	neo := bc.contracts.NEO
	tx := transaction.New(netmode.UnitTestNet, []byte{byte(opcode.PUSH1)}, 0)
	ic := bc.newInteropContext(trigger.Application, bc.dao, nil, tx)
	ic.SpawnVM()
	ic.Block = bc.newBlock(tx)

	freq := testchain.ValidatorsCount + testchain.CommitteeSize()
	advanceChain := func(t *testing.T) {
		for i := 0; i < freq; i++ {
			require.NoError(t, bc.AddBlock(bc.newBlock()))
			ic.Block.Index++
		}
	}

	standBySorted := bc.GetStandByValidators()
	sort.Sort(standBySorted)
	pubs, err := neo.ComputeNextBlockValidators(bc, ic.DAO)
	require.NoError(t, err)
	require.Equal(t, standBySorted, pubs)

	sz := testchain.CommitteeSize()
	accs := make([]*wallet.Account, sz)
	candidates := make(keys.PublicKeys, sz)
	txs := make([]*transaction.Transaction, 0, len(accs))
	for i := 0; i < sz; i++ {
		priv, err := keys.NewPrivateKey()
		require.NoError(t, err)
		candidates[i] = priv.PublicKey()
		accs[i], err = wallet.NewAccount()
		require.NoError(t, err)
		if i > 0 {
			require.NoError(t, neo.RegisterCandidateInternal(ic, candidates[i]))
		}

		to := accs[i].Contract.ScriptHash()
		w := io.NewBufBinWriter()
		emit.AppCallWithOperationAndArgs(w.BinWriter, bc.contracts.NEO.Hash, "transfer",
			neoOwner.BytesBE(), to.BytesBE(),
			big.NewInt(int64(sz-i)*1000000).Int64(), nil)
		emit.Opcodes(w.BinWriter, opcode.ASSERT)
		emit.AppCallWithOperationAndArgs(w.BinWriter, bc.contracts.GAS.Hash, "transfer",
			neoOwner.BytesBE(), to.BytesBE(),
			int64(1_000_000_000), nil)
		emit.Opcodes(w.BinWriter, opcode.ASSERT)
		require.NoError(t, w.Err)
		tx := transaction.New(netmode.UnitTestNet, w.Bytes(), 1000_000_000)
		tx.ValidUntilBlock = bc.BlockHeight() + 1
		setSigner(tx, testchain.MultisigScriptHash())
		require.NoError(t, signTx(bc, tx))
		txs = append(txs, tx)
	}
	require.NoError(t, bc.AddBlock(bc.newBlock(txs...)))
	for _, tx := range txs {
		checkTxHalt(t, bc, tx.Hash())
	}
	transferBlock := bc.BlockHeight()

	for i := 1; i < sz; i++ {
		priv := accs[i].PrivateKey()
		h := priv.GetScriptHash()
		setSigner(tx, h)
		ic.VM.Load(priv.PublicKey().GetVerificationScript())
		require.NoError(t, neo.VoteInternal(ic, h, candidates[i]))
	}

	// We still haven't voted enough validators in.
	pubs, err = neo.ComputeNextBlockValidators(bc, ic.DAO)
	require.NoError(t, err)
	require.Equal(t, standBySorted, pubs)

	advanceChain(t)
	pubs = neo.GetNextBlockValidatorsInternal()
	require.EqualValues(t, standBySorted, pubs)

	// Register and give some value to the last validator.
	require.NoError(t, neo.RegisterCandidateInternal(ic, candidates[0]))
	priv := accs[0].PrivateKey()
	h := priv.GetScriptHash()
	setSigner(tx, h)
	ic.VM.Load(priv.PublicKey().GetVerificationScript())
	require.NoError(t, neo.VoteInternal(ic, h, candidates[0]))

	ic.DAO.Persist()
	advanceChain(t)
	pubs, err = neo.ComputeNextBlockValidators(bc, ic.DAO)
	require.NoError(t, err)
	sortedCandidates := candidates.Copy()[:testchain.Size()]
	sort.Sort(sortedCandidates)
	require.EqualValues(t, sortedCandidates, pubs)

	pubs = neo.GetNextBlockValidatorsInternal()
	require.EqualValues(t, sortedCandidates, pubs)

	t.Run("check voter rewards", func(t *testing.T) {
		gasBalance := make([]*big.Int, len(accs))
		neoBalance := make([]*big.Int, len(accs))
		txs := make([]*transaction.Transaction, 0, len(accs))
		for i := range accs {
			w := io.NewBufBinWriter()
			h := accs[i].PrivateKey().GetScriptHash()
			gasBalance[i] = bc.GetUtilityTokenBalance(h)
			neoBalance[i], _ = bc.GetGoverningTokenBalance(h)
			emit.AppCallWithOperationAndArgs(w.BinWriter, bc.contracts.NEO.Hash, "transfer",
				h.BytesBE(), h.BytesBE(), int64(1), nil)
			emit.Opcodes(w.BinWriter, opcode.ASSERT)
			require.NoError(t, w.Err)
			tx := transaction.New(netmode.UnitTestNet, w.Bytes(), 0)
			tx.ValidUntilBlock = bc.BlockHeight() + 1
			tx.NetworkFee = 2_000_000
			tx.SystemFee = 10_000_000
			setSigner(tx, h)
			require.NoError(t, accs[i].SignTx(tx))
			txs = append(txs, tx)
		}
		require.NoError(t, bc.AddBlock(bc.newBlock(txs...)))
		for _, tx := range txs {
			checkTxHalt(t, bc, tx.Hash())
		}

		// GAS increase consists of 2 parts: NEO holding + voting for committee nodes.
		// Here we check that 2-nd part exists and is proportional to the amount of NEO given.
		for i := range accs {
			newGAS := bc.GetUtilityTokenBalance(accs[i].Contract.ScriptHash())
			newGAS.Sub(newGAS, gasBalance[i])

			gasForHold, err := bc.contracts.NEO.CalculateNEOHolderReward(bc.dao, neoBalance[i], transferBlock, bc.BlockHeight())
			require.NoError(t, err)
			newGAS.Sub(newGAS, gasForHold)
			require.True(t, newGAS.Sign() > 0)
			gasBalance[i] = newGAS
		}
		// First account voted later than the others.
		require.Equal(t, -1, gasBalance[0].Cmp(gasBalance[1]))
		for i := 2; i < testchain.ValidatorsCount; i++ {
			require.Equal(t, 0, gasBalance[i].Cmp(gasBalance[1]))
		}
		require.Equal(t, 1, gasBalance[1].Cmp(gasBalance[testchain.ValidatorsCount]))
		for i := testchain.ValidatorsCount; i < testchain.CommitteeSize(); i++ {
			require.Equal(t, 0, gasBalance[i].Cmp(gasBalance[testchain.ValidatorsCount]))
		}
	})

	require.NoError(t, neo.UnregisterCandidateInternal(ic, candidates[0]))
	require.Error(t, neo.VoteInternal(ic, h, candidates[0]))
	advanceChain(t)

	pubs, err = neo.ComputeNextBlockValidators(bc, ic.DAO)
	require.NoError(t, err)
	for i := range pubs {
		require.NotEqual(t, candidates[0], pubs[i])
	}
}

func TestNEO_SetGasPerBlock(t *testing.T) {
	bc := newTestChain(t)
	defer bc.Close()

	neo := bc.contracts.NEO
	tx := transaction.New(netmode.UnitTestNet, []byte{}, 0)
	ic := bc.newInteropContext(trigger.Application, bc.dao, nil, tx)
	ic.SpawnVM()
	ic.VM.LoadScript([]byte{byte(opcode.RET)})

	h := neo.GetCommitteeAddress()
	t.Run("Default", func(t *testing.T) {
		g := neo.GetGASPerBlock(ic.DAO, 0)
		require.EqualValues(t, 5*native.GASFactor, g.Int64())
	})
	t.Run("Invalid", func(t *testing.T) {
		t.Run("InvalidSignature", func(t *testing.T) {
			setSigner(tx, util.Uint160{})
			ok, err := neo.SetGASPerBlock(ic, 10, big.NewInt(native.GASFactor))
			require.NoError(t, err)
			require.False(t, ok)
		})
		t.Run("TooBigValue", func(t *testing.T) {
			setSigner(tx, h)
			_, err := neo.SetGASPerBlock(ic, 10, big.NewInt(10*native.GASFactor+1))
			require.Error(t, err)
		})
	})
	t.Run("Valid", func(t *testing.T) {
		setSigner(tx, h)
		ok, err := neo.SetGASPerBlock(ic, 10, big.NewInt(native.GASFactor*2))
		require.NoError(t, err)
		require.True(t, ok)
		neo.OnPersistEnd(ic.DAO)
		_, err = ic.DAO.Persist()
		require.NoError(t, err)

		t.Run("Again", func(t *testing.T) {
			setSigner(tx, h)
			ok, err := neo.SetGASPerBlock(ic, 10, big.NewInt(native.GASFactor))
			require.NoError(t, err)
			require.True(t, ok)

			t.Run("NotPersisted", func(t *testing.T) {
				g := neo.GetGASPerBlock(bc.dao, 10)
				// Old value should be returned.
				require.EqualValues(t, 2*native.GASFactor, g.Int64())
			})
		})

		neo.OnPersistEnd(ic.DAO)
		g := neo.GetGASPerBlock(ic.DAO, 9)
		require.EqualValues(t, 5*native.GASFactor, g.Int64())

		g = neo.GetGASPerBlock(ic.DAO, 10)
		require.EqualValues(t, native.GASFactor, g.Int64())
	})
}

func TestNEO_CalculateBonus(t *testing.T) {
	bc := newTestChain(t)
	defer bc.Close()

	neo := bc.contracts.NEO
	tx := transaction.New(netmode.UnitTestNet, []byte{}, 0)
	ic := bc.newInteropContext(trigger.Application, bc.dao, nil, tx)
	ic.SpawnVM()
	ic.VM.LoadScript([]byte{byte(opcode.RET)})
	t.Run("Invalid", func(t *testing.T) {
		_, err := neo.CalculateNEOHolderReward(ic.DAO, new(big.Int).SetInt64(-1), 0, 1)
		require.Error(t, err)
	})
	t.Run("Zero", func(t *testing.T) {
		res, err := neo.CalculateNEOHolderReward(ic.DAO, big.NewInt(0), 0, 100)
		require.NoError(t, err)
		require.EqualValues(t, 0, res.Int64())
	})
	t.Run("ManyBlocks", func(t *testing.T) {
		setSigner(tx, neo.GetCommitteeAddress())
		ok, err := neo.SetGASPerBlock(ic, 10, big.NewInt(1*native.GASFactor))
		require.NoError(t, err)
		require.True(t, ok)

		res, err := neo.CalculateNEOHolderReward(ic.DAO, big.NewInt(100), 5, 15)
		require.NoError(t, err)
		require.EqualValues(t, (100*5*5/10)+(100*5*1/10), res.Int64())

	})
}

func TestNEO_CommitteeBountyOnPersist(t *testing.T) {
	bc := newTestChain(t)
	defer bc.Close()

	hs := make([]util.Uint160, testchain.CommitteeSize())
	for i := range hs {
		hs[i] = testchain.PrivateKeyByID(i).GetScriptHash()
	}

	const singleBounty = 50000000
	bs := map[int]int64{0: singleBounty}
	checkBalances := func() {
		for i := 0; i < testchain.CommitteeSize(); i++ {
			require.EqualValues(t, bs[i], bc.GetUtilityTokenBalance(hs[i]).Int64(), i)
		}
	}
	for i := 0; i < testchain.CommitteeSize()*2; i++ {
		require.NoError(t, bc.AddBlock(bc.newBlock()))
		bs[(i+1)%testchain.CommitteeSize()] += singleBounty
		checkBalances()
	}
}

func TestNEO_TransferOnPayment(t *testing.T) {
	bc := newTestChain(t)
	defer bc.Close()

	cs, _ := getTestContractState()
	require.NoError(t, bc.dao.PutContractState(cs))

	const amount = 2
	tx := newNEP5Transfer(bc.contracts.NEO.Hash, neoOwner, cs.ScriptHash(), amount)
	tx.SystemFee += 1_000_000
	tx.NetworkFee = 10_000_000
	tx.ValidUntilBlock = bc.BlockHeight() + 1
	addSigners(tx)
	require.NoError(t, signTx(bc, tx))
	require.NoError(t, bc.AddBlock(bc.newBlock(tx)))

	aer, err := bc.GetAppExecResults(tx.Hash(), trigger.Application)
	require.NoError(t, err)
	require.Equal(t, vm.HaltState, aer[0].VMState)
	require.Len(t, aer[0].Events, 3) // transfer + auto GAS claim + onPayment

	e := aer[0].Events[2]
	require.Equal(t, "LastPayment", e.Name)
	arr := e.Item.Value().([]stackitem.Item)
	require.Equal(t, neoOwner.BytesBE(), arr[0].Value())
	require.Equal(t, big.NewInt(amount), arr[1].Value())
}
