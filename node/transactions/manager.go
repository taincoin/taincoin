package transactions

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/taincoin/taincoin/lib"
	"github.com/taincoin/taincoin/lib/utils"
	"github.com/taincoin/taincoin/lib/wallet"
	"github.com/taincoin/taincoin/node/blockchain"
	"github.com/taincoin/taincoin/node/database"
	"github.com/taincoin/taincoin/node/structures"
)

type txManager struct {
	DB     database.DBManager
	Logger *utils.LoggerMan
}

func NewManager(DB database.DBManager, Logger *utils.LoggerMan) TransactionsManagerInterface {
	return &txManager{DB, Logger}
}

// Create tx index object to use in this package
func (n txManager) getIndexManager() *transactionsIndex {
	return newTransactionIndex(n.DB, n.Logger)
}

// Create unapproved tx manage object to use in this package
func (n txManager) getUnapprovedTransactionsManager() *unApprovedTransactions {
	return &unApprovedTransactions{n.DB, n.Logger}
}

// Create unspent outputx manage object to use in this package
func (n txManager) getUnspentOutputsManager() *unspentTransactions {
	return &unspentTransactions{n.DB, n.Logger}
}

// Reindex caches
func (n *txManager) ReindexData() (map[string]int, error) {
	err := n.getIndexManager().Reindex()

	if err != nil {
		return nil, err
	}

	count, err := n.getUnspentOutputsManager().Reindex()

	if err != nil {
		return nil, err
	}

	info := map[string]int{"unspentoutputs": count}

	return info, nil
}

// Calculates balance of address. Uses DB of unspent trasaction outputs
// and cache of pending transactions
func (n *txManager) GetAddressBalance(address string) (wallet.WalletBalance, error) {
	balance := wallet.WalletBalance{}

	n.Logger.Trace.Printf("Get balance %s", address)
	result, err := n.getUnspentOutputsManager().GetAddressBalance(address)

	if err != nil {
		n.Logger.Trace.Printf("Error 1 %s", err.Error())
		return balance, err
	}

	balance.Approved = result

	// get pending
	n.Logger.Trace.Printf("Get pending %s", address)
	p, err := n.getAddressPendingBalance(address)

	if err != nil {
		n.Logger.Trace.Printf("Error 2 %s", err.Error())
		return balance, err
	}
	balance.Pending = p

	balance.Total = balance.Approved + balance.Pending

	return balance, nil
}

// return count of transactions in pool
func (n *txManager) GetUnapprovedCount() (int, error) {
	return n.getUnapprovedTransactionsManager().GetCount()
}

// return count of unspent outputs
func (n *txManager) GetUnspentCount() (int, error) {
	return n.getUnspentOutputsManager().CountUnspentOutputs()
}

// return number of unapproved transactions for new block. detect conflicts
// if there are less, it returns less than requested
func (n *txManager) GetUnapprovedTransactionsForNewBlock(number int) ([]*structures.Transaction, error) {
	txlist, err := n.getUnapprovedTransactionsManager().GetTransactions(number)

	n.Logger.Trace.Printf("Found %d transaction to mine\n", len(txlist))

	txs := []*structures.Transaction{}

	for _, tx := range txlist {
		n.Logger.Trace.Printf("Go to verify: %x\n", tx.ID)

		// we need to verify each transaction
		// we will do full deep check of transaction
		// also, a transaction can have input from other transaction from thi block
		vtx, err := n.VerifyTransaction(tx, txs, []byte{})

		if err != nil {
			// this can be case when a transaction is based on other unapproved transaction
			// and that transaction was created in same second
			n.Logger.Trace.Printf("Ignore transaction %x. Verify failed with error: %s\n", tx.ID, err.Error())
			// we delete this transaction. no sense to keep it
			n.CancelTransaction(tx.ID)
			continue
		}

		if vtx {
			// transaction is valid
			txs = append(txs, tx)
		} else {
			// the transaction is invalid. some input was already used in other confirmed transaction
			// or somethign wrong with signatures.
			// remove this transaction from the DB of unconfirmed transactions
			n.Logger.Trace.Printf("Delete transaction used in other block before: %x\n", tx.ID)
			n.CancelTransaction(tx.ID)
		}
	}
	txlist = nil

	n.Logger.Trace.Printf("After verification %d transaction are left\n", len(txs))

	if len(txs) == 0 {
		return nil, errors.New("All transactions are invalid! Waiting for new ones...")
	}

	// now it is needed to check if transactions don't conflict one to other
	var badtransactions []*structures.Transaction
	txs, badtransactions, err = n.getUnapprovedTransactionsManager().DetectConflicts(txs)

	n.Logger.Trace.Printf("After conflict detection %d - fine, %d - conflicts\n", len(txs), len(badtransactions))

	if err != nil {
		return nil, err
	}

	if len(badtransactions) > 0 {
		// there are conflicts! remove conflicting transactions
		for _, tx := range badtransactions {
			n.Logger.Trace.Printf("Delete conflicting transaction: %x\n", tx.ID)
			n.CancelTransaction(tx.ID)
		}
	}
	return txs, nil
}

/*
* Cancels unapproved transaction.
* NOTE this can work only for local node. it a transaction was already sent to other nodes, it will not be canceled
* and can be added to next block
 */
func (n *txManager) CancelTransaction(txid []byte) error {

	found, err := n.getUnapprovedTransactionsManager().Delete(txid)

	if err == nil && !found {
		return errors.New("Transaction ID not found in the list of unapproved transactions")
	}

	return nil
}

// Verify if transaction is correct.
// If it is build on correct outputs.This does checks agains blockchain. Needs more time
// NOTE Transaction can have outputs of other transactions that are not yet approved.
// This must be considered as correct case
func (n *txManager) VerifyTransaction(tx *structures.Transaction, prevtxs []*structures.Transaction, tip []byte) (bool, error) {
	inputTXs, notFoundInputs, err := n.getInputTransactionsState(tx, tip)
	if err != nil {
		return false, err
	}

	if len(notFoundInputs) > 0 {
		// some of inputs can be from other transactions in this pool
		inputTXs, err = n.getUnapprovedTransactionsManager().CheckInputsWereBefore(notFoundInputs, prevtxs, inputTXs)

		if err != nil {
			return false, err
		}
	}
	// do final check against inputs

	err = tx.Verify(inputTXs)

	if err != nil {
		return false, err
	}

	return true, nil
}

// Iterate over unapproved transactions, for example to display them . Accepts callback as argument
func (n *txManager) ForEachUnapprovedTransaction(callback UnApprovedTransactionCallbackInterface) (int, error) {
	return n.getUnapprovedTransactionsManager().forEachUnapprovedTransaction(callback)
}

// Iterate over unspent transactions outputs, for example to display them . Accepts callback as argument
func (n *txManager) ForEachUnspentOutput(address string, callback UnspentTransactionOutputCallbackInterface) error {
	return n.getUnspentOutputsManager().forEachUnspentOutput(address, callback)
}

// Remove all transactions from unapproved cache (transactions pool)
func (n *txManager) CleanUnapprovedCache() error {
	return n.getUnapprovedTransactionsManager().CleanUnapprovedCache()
}

// to execute when new block added . the block must not be on top
func (n *txManager) BlockAdded(block *structures.Block, ontopofchain bool) error {
	// update caches
	n.getIndexManager().BlockAdded(block)

	if ontopofchain {
		n.getUnapprovedTransactionsManager().DeleteFromBlock(block)
		n.getUnspentOutputsManager().UpdateOnBlockAdd(block)
	}

	return nil
}

// Block was removed from the top of primary blockchain branch
func (n *txManager) BlockRemoved(block *structures.Block) error {
	n.getUnapprovedTransactionsManager().AddFromCanceled(block.Transactions)
	n.getUnspentOutputsManager().UpdateOnBlockCancel(block)
	n.getIndexManager().BlockRemoved(block)
	return nil
}

// block is now added to primary chain. it existed in DB before
func (n *txManager) BlockAddedToPrimaryChain(block *structures.Block) error {
	n.getUnapprovedTransactionsManager().DeleteFromBlock(block)
	n.getUnspentOutputsManager().UpdateOnBlockAdd(block)
	return nil
}

// block is removed from primary chain. it continued to be in DB on side branch
func (n *txManager) BlockRemovedFromPrimaryChain(block *structures.Block) error {
	n.getUnapprovedTransactionsManager().AddFromCanceled(block.Transactions)
	n.getUnspentOutputsManager().UpdateOnBlockCancel(block)
	return nil
}

// Send amount of money if a node is not running.
// This function only adds a transaction to queue
// Attempt to send the transaction to other nodes will be done in other place
//
// Returns new transaction hash. This return can be used to try to send transaction
// to other nodes or to try mining
func (n *txManager) CreateTransaction(PubKey []byte, privKey ecdsa.PrivateKey, to string, amount float64) (*structures.Transaction, error) {

	if amount <= 0 {
		return nil, errors.New("Amount must be positive value")
	}
	if to == "" {
		return nil, errors.New("Recipient address is not provided")
	}

	txBytes, DataToSign, err := n.PrepareNewTransaction(PubKey, to, amount)

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Prepare error: %s", err.Error()))
	}

	signatures, err := utils.SignDataSet(PubKey, privKey, DataToSign)

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Sign error: %s", err.Error()))
	}
	NewTX, err := n.ReceivedNewTransactionData(txBytes, signatures)

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Final ading TX error: %s", err.Error()))
	}

	return NewTX, nil
}

// New transactions created. It is received in serialysed view and signatures separately
// This data is ready to be convertd to complete gransaction
func (n *txManager) ReceivedNewTransactionData(txBytes []byte, Signatures [][]byte) (*structures.Transaction, error) {
	tx := structures.Transaction{}
	err := tx.DeserializeTransaction(txBytes)

	if err != nil {
		return nil, err
	}

	err = tx.SetSignatures(Signatures)

	if err != nil {
		return nil, err
	}

	err = n.ReceivedNewTransaction(&tx)

	if err != nil {
		return nil, err
	}

	return &tx, nil
}

// New transaction reveived from other node. We need to verify and add to cache of unapproved
func (n *txManager) ReceivedNewTransaction(tx *structures.Transaction) error {
	// verify this transaction
	good, err := n.verifyTransactionQuick(tx)

	if err != nil {
		return err
	}
	if !good {
		return errors.New("Transaction verification failed")
	}
	// if all is ok, add it to the list of unapproved
	return n.getUnapprovedTransactionsManager().Add(tx)
}

// Request to make new transaction and prepare data to sign
// This function should find good input transactions for this amount
// Including inputs from unapproved transactions if no good approved transactions yet
func (n *txManager) PrepareNewTransaction(PubKey []byte, to string, amount float64) ([]byte, [][]byte, error) {
	amount, err := strconv.ParseFloat(fmt.Sprintf("%.8f", amount), 64)

	if err != nil {
		return nil, nil, err
	}
	PubKeyHash, _ := utils.HashPubKey(PubKey)
	// get from pending transactions. find outputs used by this pubkey
	pendinginputs, pendingoutputs, _, err := n.getUnapprovedTransactionsManager().GetPreparedBy(PubKeyHash)
	n.Logger.Trace.Printf("Pending transactions state: %d- inputs, %d - unspent outputs", len(pendinginputs), len(pendingoutputs))

	inputs, prevTXs, totalamount, err := n.getUnspentOutputsManager().GetNewTransactionInputs(PubKey, to, amount, pendinginputs)

	if err != nil {
		return nil, nil, err
	}

	n.Logger.Trace.Printf("First step prepared amount %f of %f", totalamount, amount)

	if totalamount < amount {
		// no anough funds in confirmed transactions
		// pending must be used

		if len(pendingoutputs) == 0 {
			// nothing to add
			return nil, nil, errors.New("No enough funds for requested transaction")
		}
		inputs, prevTXs, totalamount, err =
			n.getUnspentOutputsManager().ExtendNewTransactionInputs(PubKey, amount, totalamount,
				inputs, prevTXs, pendingoutputs)

		if err != nil {
			return nil, nil, err
		}
	}

	n.Logger.Trace.Printf("Second step prepared amount %f of %f", totalamount, amount)

	if totalamount < amount {
		return nil, nil, errors.New("No anough funds to make new transaction")
	}

	return n.prepareNewTransactionComplete(PubKey, to, amount, inputs, totalamount, prevTXs)
}

//
func (n *txManager) prepareNewTransactionComplete(PubKey []byte, to string, amount float64,
	inputs []structures.TXInput, totalamount float64, prevTXs map[string]structures.Transaction) ([]byte, [][]byte, error) {

	var outputs []structures.TXOutput

	// Build a list of outputs
	from, _ := utils.PubKeyToAddres(PubKey)
	outputs = append(outputs, *structures.NewTXOutput(amount, to))

	if totalamount > amount && totalamount-amount > lib.SmallestUnit {
		outputs = append(outputs, *structures.NewTXOutput(totalamount-amount, from)) // a change
	}

	inputTXs := make(map[int]*structures.Transaction)

	for vinInd, vin := range inputs {
		tx := prevTXs[hex.EncodeToString(vin.Txid)]
		inputTXs[vinInd] = &tx
	}

	tx := structures.Transaction{nil, inputs, outputs, 0}
	tx.TimeNow()

	signdata, err := tx.PrepareSignData(inputTXs)

	if err != nil {
		return nil, nil, err
	}

	txBytes, err := tx.Serialize()

	if err != nil {
		return nil, nil, err
	}

	return txBytes, signdata, nil
}

// check if transaction exists. it checks in all places. in approved and pending
func (n *txManager) GetIfExists(txid []byte) (*structures.Transaction, error) {
	// check in pending first
	tx, err := n.getUnapprovedTransactionsManager().GetIfExists(txid)

	if !(tx == nil && err == nil) {
		return tx, err
	}

	// not exist on pending and no error
	// try to check in approved . it will look only in primary branch
	tx, _, _, err = n.getIndexManager().GetTransactionAllInfo(txid, []byte{})
	return tx, err
}

// check if transaction exists in unapproved cache
func (n *txManager) GetIfUnapprovedExists(txid []byte) (*structures.Transaction, error) {
	// check in pending first
	tx, err := n.getUnapprovedTransactionsManager().GetIfExists(txid)

	if !(tx == nil && err == nil) {
		return tx, err
	}
	return nil, nil
}

// Calculates pending balance of address.
func (n *txManager) getAddressPendingBalance(address string) (float64, error) {
	PubKeyHash, _ := utils.AddresToPubKeyHash(address)

	// inputs this is what a wallet spent from his real approved balance
	// outputs this is what a wallet receives (and didn't resulse in other pending TXs)
	// slice inputs contains only inputs from approved transactions outputs
	_, outputs, inputs, err := n.getUnapprovedTransactionsManager().GetPreparedBy(PubKeyHash)

	if err != nil {
		return 0, err
	}

	pendingbalance := float64(0)

	for _, o := range outputs {
		// this is amount sent to this wallet and this
		// list contains only what was not spent in other prepared TX
		pendingbalance += o.Value
	}

	// we need to know values for inputs. this are inputs based on TXs that are in approved
	// input TX can be confirmed (in unspent outputs) or unconfirmed . we need to look for it in
	// both places
	for _, i := range inputs {
		n.Logger.Trace.Printf("find input %s for tx %x", i, i.Txid)
		v, err := n.getUnspentOutputsManager().GetInputValue(i)

		if err != nil {
			/*
				if err, ok := err.(*TXNotFoundError); ok && err.GetKind() == TXNotFoundErrorUnspent {

					// check this TX in prepared
					v2, err := n.GetUnapprovedTransactionsManager().GetInputValue(i)

					if err != nil {
						return 0, errors.New(fmt.Sprintf("Pending Balance Error: input check fails on unapproved: %s", err.Error()))
					}
					v = v2
				} else {
					return 0, errors.New(fmt.Sprintf("Pending Balance Error: input check fails on unspent: %s", err.Error()))
				}
			*/
			return 0, errors.New(fmt.Sprintf("Pending Balance Error: input check fails on unspent: %s", err.Error()))
		}
		pendingbalance -= v
	}

	return pendingbalance, nil
}

// Verify if transaction is correct.
// If it is build on correct outputs.It checks only cache of unspent transactions
// This function doesn't do full alidation with blockchain
// NOTE Transaction can have outputs of other transactions that are not yet approved.
// This must be considered as correct case
func (n *txManager) verifyTransactionQuick(tx *structures.Transaction) (bool, error) {
	notFoundInputs, inputTXs, err := n.getUnspentOutputsManager().VerifyTransactionsOutputsAreNotSpent(tx.Vin)

	if err != nil {
		n.Logger.Trace.Printf("VT error 1: %s", err.Error())
		return false, err
	}

	if len(notFoundInputs) > 0 {
		// some inputs are not existent
		// we need to try to find them in list of unapproved transactions
		// if not found then it is bad transaction
		err := n.getUnapprovedTransactionsManager().CheckInputsArePrepared(notFoundInputs, inputTXs)

		if err != nil {
			return false, err
		}
	}
	// verify signatures

	err = tx.Verify(inputTXs)

	if err != nil {
		return false, err
	}
	return true, nil
}

// Verifies transaction inputs. Check if that are real existent transactions. And that outputs are not yet used
// Is some transaction is not in blockchain, returns nil pointer in map and this input in separate map
// Missed inputs can be some unconfirmed transactions
// Returns: map of previous transactions (full info about input TX). map by input index
// next map is wrong input, where a TX is not found.
func (n *txManager) getInputTransactionsState(tx *structures.Transaction,
	tip []byte) (map[int]*structures.Transaction, map[int]structures.TXInput, error) {

	//n.Logger.Trace.Printf("get state %x , tip %x", tx.ID, tip)

	prevTXs := make(map[int]*structures.Transaction)
	badinputs := make(map[int]structures.TXInput)

	if tx.IsCoinbase() {

		return prevTXs, badinputs, nil
	}

	bcMan, err := blockchain.NewBlockchainManager(n.DB, n.Logger)

	if err != nil {
		return nil, nil, err
	}

	for vind, vin := range tx.Vin {
		//n.Logger.Trace.Printf("Load in tx %x", vin.Txid)
		txBockHashes, err := n.getIndexManager().GetTranactionBlocks(vin.Txid)

		if err != nil {
			n.Logger.Trace.Printf("Error %s", err.Error())
			return nil, nil, err
		}

		txBockHash, err := bcMan.ChooseHashUnderTip(txBockHashes, tip)

		if err != nil {
			n.Logger.Trace.Printf("Error getting correct hash %s", err.Error())
			return nil, nil, err
		}

		var prevTX *structures.Transaction

		if txBockHash == nil {
			//n.Logger.Trace.Printf("Not found TX")
			prevTX = nil
		} else {

			// if block is in this chain
			//n.Logger.Trace.Printf("block height %d", heigh)
			prevTX, err = bcMan.GetTransactionFromBlock(vin.Txid, txBockHash)

			if err != nil {
				return nil, nil, err
			}

		}

		if prevTX == nil {
			// transaction not found
			badinputs[vind] = vin
			prevTXs[vind] = nil
			//n.Logger.Trace.Printf("tx is not in blocks")
		} else {
			//n.Logger.Trace.Printf("tx found")
			// check if this input was not yet spent somewhere
			spentouts, err := n.getIndexManager().GetTranactionOutputsSpent(vin.Txid, txBockHash, tip)

			if err != nil {
				return nil, nil, err
			}
			//n.Logger.Trace.Printf("spending of tx count %d", len(spentouts))
			if len(spentouts) > 0 {

				for _, o := range spentouts {
					if o.OutInd == vin.Vout {

						return nil, nil, errors.New("Transaction input was already spent before")
					}
				}
			}
			// the transaction out was not yet spent
			prevTXs[vind] = prevTX
		}
	}

	return prevTXs, badinputs, nil
}
