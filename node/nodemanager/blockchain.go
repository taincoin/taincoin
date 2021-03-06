package nodemanager

import (
	"errors"

	"github.com/taincoin/taincoin/lib/utils"
	"github.com/taincoin/taincoin/lib/wallet"
	"github.com/taincoin/taincoin/node/blockchain"
	"github.com/taincoin/taincoin/node/consensus"
	"github.com/taincoin/taincoin/node/structures"
	"github.com/taincoin/taincoin/node/transactions"
)

type NodeBlockchain struct {
	Logger        *utils.LoggerMan
	MinterAddress string
	DBConn        *Database
}

func (n *NodeBlockchain) GetBCManager() *blockchain.Blockchain {
	bcm, _ := blockchain.NewBlockchainManager(n.DBConn.DB(), n.Logger)
	return bcm
}

func (n *NodeBlockchain) getTransactionsManager() transactions.TransactionsManagerInterface {
	return transactions.NewManager(n.DBConn.DB(), n.Logger)
}

// Checks if a block exists in the chain. It will go over blocks list
func (n *NodeBlockchain) CheckBlockExists(blockHash []byte) (bool, error) {
	return n.GetBCManager().CheckBlockExists(blockHash)
}

// Get block objet by hash
func (n *NodeBlockchain) GetBlock(hash []byte) (*structures.Block, error) {
	block, err := n.GetBCManager().GetBlock(hash)

	return &block, err
}

// Returns height of the chain. Index of top block
func (n *NodeBlockchain) GetBestHeight() (int, error) {
	bcm := n.GetBCManager()

	bestHeight, err := bcm.GetBestHeight()

	if err != nil {
		return 0, err
	}

	return bestHeight, nil
}

// Return top hash
func (n *NodeBlockchain) GetTopBlockHash() ([]byte, error) {
	bcm := n.GetBCManager()

	topHash, _, err := bcm.GetState()

	if err != nil {
		return nil, err
	}

	return topHash, nil
}

// Returns history of transactions for given address
func (n *NodeBlockchain) GetAddressHistory(address string) ([]structures.TransactionsHistory, error) {
	if address == "" {
		return nil, errors.New("Address is missed")
	}
	w := wallet.Wallet{}

	if !w.ValidateAddress(address) {
		return nil, errors.New("Address is not valid")
	}
	bci, err := blockchain.NewBlockchainIterator(n.DBConn.DB())

	if err != nil {
		return nil, err
	}

	pubKeyHash, _ := utils.AddresToPubKeyHash(address)

	return bci.GetAddressHistory(pubKeyHash, address)
}

// Drop block from a top of blockchain
func (n *NodeBlockchain) DropBlock() (*structures.Block, error) {
	return n.GetBCManager().DeleteBlock()
}

// Add block to blockchain
// Block is not yet verified
func (n *NodeBlockchain) AddBlock(block *structures.Block) (uint, error) {
	// do some checks of the block
	// check if block exists
	blockstate, err := n.CheckBlockState(block.Hash, block.PrevBlockHash)

	if err != nil {
		return 0, err
	}

	if blockstate == 1 {
		// block exists. no sese to continue
		return blockchain.BCBAddState_notAddedExists, nil
	}

	if blockstate == 2 {
		// previous bock is not found. can not add
		return blockchain.BCBAddState_notAddedNoPrev, nil
	}

	Minter, err := consensus.NewConsensusManager(n.MinterAddress, n.DBConn.DB(), n.Logger)

	if err != nil {
		return 0, err
	}
	// verify this block against rules.
	err = Minter.VerifyBlock(block)

	if err != nil {
		return 0, err
	}

	return n.GetBCManager().AddBlock(block)
}

// returns two branches of a block starting from their common block.
// One of branches is primary at this time
func (n *NodeBlockchain) GetBranchesReplacement(sideBranchHash []byte, tip []byte) ([]*structures.Block, []*structures.Block, error) {
	bcm := n.GetBCManager()

	if len(tip) == 0 {
		tip, _, _ = bcm.GetState()
	}
	return bcm.GetBranchesReplacement(sideBranchHash, tip)
}

/*
* Checks state of a block by hashes
* returns
* -1 BCBState_error
* 0 BCBState_canAdd if block doesn't exist and prev block exists
* 1 BCBState_exists if block exists
* 2 BCBState_notExistAndPrevNotExist if block doesn't exist and prev block doesn't exist
 */
func (n *NodeBlockchain) CheckBlockState(hash, prevhash []byte) (int, error) {
	exists, err := n.CheckBlockExists(hash)

	if err != nil {
		return -1, err
	}

	if exists {
		return 1, nil
	}
	exists, err = n.CheckBlockExists(prevhash)

	if err != nil {
		return -1, err
	}

	if exists {
		return 0, nil
	}

	return 2, nil
}

// Get next blocks uppper then given
func (n *NodeBlockchain) GetBlocksAfter(hash []byte) ([]*structures.BlockShort, error) {
	exists, err := n.CheckBlockExists(hash)

	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, nil // nil for blocks list means given hash is not found
	}

	// there are 2 cases: block is in main branch , and it is not in main branch
	// this will be nil if a hash is not in a chain

	return n.GetBCManager().GetNextBlocks(hash)
}
