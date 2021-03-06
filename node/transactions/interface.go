package transactions

import (
	"crypto/ecdsa"

	"github.com/taincoin/taincoin/lib/wallet"
	"github.com/taincoin/taincoin/node/structures"
)

type UnApprovedTransactionCallbackInterface func(txhash, txstr string) error
type UnspentTransactionOutputCallbackInterface func(fromaddr string, value float64, txID []byte, output int, isbase bool) error

type TransactionsManagerInterface interface {
	GetAddressBalance(address string) (wallet.WalletBalance, error)
	GetUnapprovedCount() (int, error)
	GetUnspentCount() (int, error)
	GetUnapprovedTransactionsForNewBlock(number int) ([]*structures.Transaction, error)
	GetIfExists(txid []byte) (*structures.Transaction, error)
	GetIfUnapprovedExists(txid []byte) (*structures.Transaction, error)

	VerifyTransaction(tx *structures.Transaction, prevtxs []*structures.Transaction, tip []byte) (bool, error)

	ForEachUnspentOutput(address string, callback UnspentTransactionOutputCallbackInterface) error
	ForEachUnapprovedTransaction(callback UnApprovedTransactionCallbackInterface) (int, error)

	// Create transaction methods
	CreateTransaction(PubKey []byte, privKey ecdsa.PrivateKey, to string, amount float64) (*structures.Transaction, error)
	ReceivedNewTransaction(tx *structures.Transaction) error
	ReceivedNewTransactionData(txBytes []byte, Signatures [][]byte) (*structures.Transaction, error)
	PrepareNewTransaction(PubKey []byte, to string, amount float64) ([]byte, [][]byte, error)

	// new block was created in blockchain DB. It must not be on top of primary blockchain
	BlockAdded(block *structures.Block, ontopofchain bool) error
	// block was removed from blockchain DB from top
	BlockRemoved(block *structures.Block) error
	// block was not in primary chain and now is
	BlockAddedToPrimaryChain(block *structures.Block) error
	// block was in primary chain and now is not
	BlockRemovedFromPrimaryChain(block *structures.Block) error

	CancelTransaction(txID []byte) error
	ReindexData() (map[string]int, error)
	CleanUnapprovedCache() error
}
