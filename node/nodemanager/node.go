package nodemanager

import (
	"crypto/ecdsa"
	"errors"
	"math/rand"
	"time"

	"github.com/taincoin/taincoin/lib/net"
	"github.com/taincoin/taincoin/lib/nodeclient"
	"github.com/taincoin/taincoin/lib/utils"
	"github.com/taincoin/taincoin/lib/wallet"
	"github.com/taincoin/taincoin/node/blockchain"
	"github.com/taincoin/taincoin/node/consensus"
	"github.com/taincoin/taincoin/node/structures"
	"github.com/taincoin/taincoin/node/transactions"
)

// This structure is central part of the application. only it can acces to blockchain and inside it all operation are done
type Node struct {
	NodeBC  NodeBlockchain
	NodeNet net.NodeNetwork
	Logger  *utils.LoggerMan
	DataDir string

	MinterAddress string
	NodeClient    *nodeclient.NodeClient
	OtherNodes    []net.NodeAddr
	DBConn        *Database
	SessionID     string
}

// Init node.
// Init interfaces of all DBs, blockchain, unspent transactions, unapproved transactions
func (n *Node) Init() {
	n.NodeNet.Init()

	n.NodeNet.Logger = n.Logger
	n.NodeBC.Logger = n.Logger

	n.NodeBC.MinterAddress = n.MinterAddress

	n.NodeBC.DBConn = n.DBConn

	// Nodes list storage
	n.NodeNet.SetExtraManager(NodesListStorage{n.DBConn, n.SessionID})
	// load list of nodes from config
	n.NodeNet.SetNodes([]net.NodeAddr{}, true)

	n.InitClient()

	rand.Seed(time.Now().UTC().UnixNano())
}

// Build transaction manager structure
func (n *Node) GetTransactionsManager() transactions.TransactionsManagerInterface {
	return transactions.NewManager(n.DBConn.DB(), n.Logger)
}

// Build BC manager structure
func (n *Node) GetBCManager() (*blockchain.Blockchain, error) {
	return blockchain.NewBlockchainManager(n.DBConn.DB(), n.Logger)
}

// Creates iterator to go over blockchain
func (n *Node) GetBlockChainIterator() (*blockchain.BlockchainIterator, error) {
	return blockchain.NewBlockchainIterator(n.DBConn.DB())
}

// Init block maker object. It is used to make new blocks
func (n *Node) getBlockMakeManager() (consensus.ConsensusInterface, error) {
	return consensus.NewConsensusManager(n.MinterAddress, n.DBConn.DB(), n.Logger)
}

// Init block maker object. It is used to make new blocks
func (n *Node) getCreateManager() *makeBlockchain {
	return &makeBlockchain{n.Logger, n.MinterAddress, n.DBConn}
}

// Init network client object. It is used to communicate with other nodes
func (n *Node) InitClient() error {
	if n.NodeClient != nil {
		return nil
	}

	client := nodeclient.NodeClient{}

	client.Logger = n.Logger
	client.NodeNet = &n.NodeNet

	n.NodeClient = &client

	return nil
}

// Load list of other nodes addresses
func (n *Node) InitNodes(list []net.NodeAddr, force bool) error {
	if n.DBConn.OpenConnectionIfNeeded("CheckNodesAndGenesis", n.SessionID) {
		defer n.DBConn.CloseConnection()
	}

	if len(list) == 0 && !force {

		n.NodeNet.LoadNodes()

		// load nodes from local storage of nodes
		if n.NodeNet.GetCountOfKnownNodes() == 0 && n.BlockchainExist() {
			// there are no any known nodes.

			bcm := n.NodeBC.GetBCManager()

			genesisHash, err := bcm.GetGenesisBlockHash()

			if err == nil {
				// load them from some external resource
				n.NodeNet.LoadInitialNodes(genesisHash)
			}

		}
	} else {
		n.NodeNet.SetNodes(list, true)
	}
	return nil
}

// Check if blockchain already exists. If no, we will not allow most of operations
// It is needed to create it first

func (n *Node) BlockchainExist() bool {

	exists, _ := n.DBConn.DB().CheckDBExists()

	// close DB. We do this check almost for any operation
	// we don't need to keep connection for evey of them
	n.DBConn.CloseConnection()

	return exists
}

// Create new blockchain, add genesis block witha given text
func (n *Node) CreateBlockchain(address, genesisCoinbaseData string) error {
	bccreator := n.getCreateManager()
	bccreator.MinterAddress = address

	return bccreator.CreateBlockchain(genesisCoinbaseData)
}

// Creates new blockchain DB from given list of blocks
// This would be used when new empty node started and syncs with other nodes

func (n *Node) InitBlockchainFromOther(host string, port int) (bool, error) {
	if host == "" {
		// load node from special hardcoded url
		n.NodeNet.LoadInitialNodes(nil)
		// get node from known nodes
		if len(n.NodeNet.Nodes) == 0 {

			return false, errors.New("No known nodes to request a blockchain")
		}
		nd := n.NodeNet.Nodes[rand.Intn(len(n.NodeNet.Nodes))]

		host = nd.Host
		port = nd.Port
	}
	addr := net.NodeAddr{host, port}

	complete, err := n.getCreateManager().InitBlockchainFromOther(addr, n.NodeClient, &n.NodeBC)

	if err != nil {
		return false, err
	}
	// add that node to list of known nodes.
	n.NodeNet.AddNodeToKnown(addr)

	return complete, nil
}

/*
* Send transaction to all known nodes. This wil send only hash and node hash to check if hash exists or no
 */
func (n *Node) SendTransactionToAll(tx *structures.Transaction) {
	n.Logger.Trace.Printf("Send transaction to %d nodes", len(n.NodeNet.Nodes))

	for _, node := range n.NodeNet.Nodes {
		if node.CompareToAddress(n.NodeClient.NodeAddress) {
			continue
		}
		n.Logger.Trace.Printf("Send TX %x to %s", tx.ID, node.NodeAddrToString())
		n.NodeClient.SendInv(node, "tx", [][]byte{tx.ID})
	}
}

// Add node
// We need this for case when we want to do some more actions after node added
func (n *Node) AddNodeToKnown(addr net.NodeAddr, sendversion bool) {
	// this is just aliace. check function will do all work
	// it will check if addres is in list, if no, it will send list of all known
	// nodes to that address and ad it to known
	added := n.CheckAddressKnown(addr)

	if added && sendversion {
		n.Logger.Trace.Printf("Added node %s\n", addr.NodeAddrToString())
		// end version to this node
		n.SendVersionToNodes([]net.NodeAddr{addr})
	}
}

// Send block to all known nodes
// This is used in case when new block was received from other node or
// created by this node. We will notify our network about new block
// But not send full block, only hash and previous hash. So, other can copy it
// Address from where we get it will be skipped
func (n *Node) SendBlockToAll(newBlock *structures.Block, skipaddr net.NodeAddr) {
	for _, node := range n.NodeNet.Nodes {
		if node.CompareToAddress(n.NodeClient.NodeAddress) {
			continue
		}
		blockshortdata, err := newBlock.GetShortCopy().Serialize()
		if err == nil {
			n.NodeClient.SendInv(node, "block", [][]byte{blockshortdata})
		}
	}
}

/*
* Send own version to all known nodes
 */
func (n *Node) SendVersionToNodes(nodes []net.NodeAddr) {
	opened := n.DBConn.OpenConnectionIfNeeded("GetHeigh", n.SessionID)
	bestHeight, err := n.NodeBC.GetBestHeight()

	if opened {
		n.DBConn.CloseConnection()
	}

	if err != nil {
		return
	}

	if len(nodes) == 0 {
		nodes = n.NodeNet.Nodes
	}

	for _, node := range nodes {
		if node.CompareToAddress(n.NodeClient.NodeAddress) {
			continue
		}
		n.NodeClient.SendVersion(node, bestHeight)
	}
}

/*
* Check if the address is known . If not then add to known
* and send list of all addresses to that node
 */
func (n *Node) CheckAddressKnown(addr net.NodeAddr) bool {
	if !n.NodeNet.CheckIsKnown(addr) {
		// send him all addresses
		n.Logger.Trace.Printf("sending list of address to %s , %s", addr.NodeAddrToString(), n.NodeNet.Nodes)
		n.NodeClient.SendAddrList(addr, n.NodeNet.Nodes)

		n.NodeNet.AddNodeToKnown(addr)

		return true
	}

	return false
}

/*
* Send money .
* This adds a transaction directly to the DB. Can be executed when a node server is not running
 */
func (n *Node) Send(PubKey []byte, privKey ecdsa.PrivateKey, to string, amount float64) ([]byte, error) {
	// get pubkey of the wallet with "from" address
	if to == "" {
		return nil, errors.New("Recipient address is not provided")
	}
	w := wallet.Wallet{}

	if !w.ValidateAddress(to) {
		return nil, errors.New("Recipient address is not valid")
	}

	tx, err := n.GetTransactionsManager().CreateTransaction(PubKey, privKey, to, amount)

	if err != nil {
		return nil, err
	}
	n.SendTransactionToAll(tx)

	return tx.ID, nil
}

// Try to make a block. If no enough transactions, send new transaction to all other nodes
func (n *Node) TryToMakeBlock(newTransactionID []byte) ([]byte, error) {
	n.Logger.Trace.Println("Try to make new block")

	w := wallet.Wallet{}

	if n.MinterAddress == "" || !w.ValidateAddress(n.MinterAddress) {
		return nil, errors.New("Minter address is not provided")
	}

	n.Logger.Trace.Println("Create block maker")
	// check how many transactions are ready to be added to a block
	Minter, _ := n.getBlockMakeManager()

	prepres, err := Minter.PrepareNewBlock()

	if err != nil {
		return nil, err
	}

	// close it while doing the proof of work
	n.DBConn.CloseConnection()
	// and close it again in the end of function
	defer n.DBConn.CloseConnection()

	if prepres != consensus.BlockPrepare_Done {
		n.Logger.Trace.Println("No anough transactions to make a block")

		if len(newTransactionID) > 1 {
			n.Logger.Trace.Printf("Send this new transaction to all other")
			// block was not created and txID is real transaction ID
			// send this transaction to all other nodes.

			tx, err := n.GetTransactionsManager().GetIfUnapprovedExists(newTransactionID)

			if err == nil && tx != nil {
				// send TX to all other nodes
				n.SendTransactionToAll(tx)
			} else if err != nil {
				n.Logger.Trace.Printf("Error: %s", err.Error())
			} else if tx == nil {
				n.Logger.Trace.Printf("Error: TX %x is not found", newTransactionID)
			}
		}

		return nil, nil
	}

	block, err := Minter.CompleteBlock()

	if err != nil {
		n.Logger.Trace.Printf("Block completion error. %s", err)
		return nil, err
	}

	n.Logger.Trace.Printf("Add block to the blockchain. Hash %x\n", block.Hash)

	// We set DB again because after close it could be update
	Minter.SetDBManager(n.DBConn.DB())

	// add new block to local blockchain. this will check a block again
	// TODO we need to skip checking. no sense, we did it right
	_, err = n.AddBlock(block)

	if err != nil {
		return nil, err
	}
	// send new block to all known nodes
	n.SendBlockToAll(block, net.NodeAddr{} /*nothing to skip*/)

	n.Logger.Trace.Println("Block done. Sent to all")

	return block.Hash, nil

}

// Add new block to blockchain.
// It can be executed when new block was created locally or received from other node

func (n *Node) AddBlock(block *structures.Block) (uint, error) {
	bcm, err := n.GetBCManager()

	if err != nil {
		return 0, err
	}

	curLastHash, _, err := bcm.GetState()

	// we need to know how the block was added to managed transactions caches correctly
	addstate, err := n.NodeBC.AddBlock(block)

	if err != nil {
		return 0, err
	}

	if addstate == blockchain.BCBAddState_addedToParallel ||
		addstate == blockchain.BCBAddState_addedToTop ||
		addstate == blockchain.BCBAddState_addedToParallelTop {

		n.GetTransactionsManager().BlockAdded(block, addstate == blockchain.BCBAddState_addedToTop)
	}

	if addstate == blockchain.BCBAddState_addedToParallelTop {
		// get 2 blocks branches that replaced each other
		newChain, oldChain, err := n.NodeBC.GetBranchesReplacement(curLastHash, []byte{})

		if err != nil {
			return 0, err
		}

		if newChain != nil && oldChain != nil {
			for _, block := range oldChain {

				err := n.GetTransactionsManager().BlockRemovedFromPrimaryChain(block)

				if err != nil {

					return 0, err
				}
			}
			for _, block := range newChain {

				err := n.GetTransactionsManager().BlockAddedToPrimaryChain(block)

				if err != nil {

					return 0, err
				}
			}
		}
	}

	return addstate, nil
}

/*
* Drop block from the top of blockchain
* This will not check if there are other branch that can now be longest and becomes main branch
 */
func (n *Node) DropBlock() error {
	block, err := n.NodeBC.DropBlock()

	if err != nil {
		return err
	}

	n.GetTransactionsManager().BlockRemoved(block)

	return nil
}

// New block info received from oher node. It is only Hash and PrevHash, not full block
// Check if this is new block and if previous block is fine
// returns state of processing. if a block data was requested or exists or prev doesn't exist
func (n *Node) ReceivedBlockFromOtherNode(addrfrom net.NodeAddr, bsdata []byte) (int, error) {

	bs := &structures.BlockShort{}
	err := bs.DeserializeBlock(bsdata)

	if err != nil {
		return 0, err
	}
	// check if block exists
	blockstate, err := n.NodeBC.CheckBlockState(bs.Hash, bs.PrevBlockHash)

	if err != nil {
		return 0, err
	}

	if blockstate == 0 {
		// in this case we can request this block full info
		n.NodeClient.SendGetData(addrfrom, "block", bs.Hash)
		return 0, nil // 0 means a block can be added and now we requested info about it
	}
	return blockstate, nil
}

/*
* New block info received from oher node
* Check if this is new block and if previous block is fine
* returns state of processing. if a block data was requested or exists or prev doesn't exist
 */
func (n *Node) ReceivedFullBlockFromOtherNode(blockdata []byte) (int, uint, *structures.Block, error) {
	addstate := uint(blockchain.BCBAddState_error)

	block := &structures.Block{}
	err := block.DeserializeBlock(blockdata)

	if err != nil {
		return -1, addstate, nil, err
	}

	n.Logger.Trace.Printf("Recevied a new block %x", block.Hash)

	// check state of this block
	blockstate, err := n.NodeBC.CheckBlockState(block.Hash, block.PrevBlockHash)

	if err != nil {
		return 0, addstate, nil, err
	}

	if blockstate == 0 {
		// only in this case we can add a block!
		// addblock should also verify the block
		addstate, err = n.AddBlock(block)

		if err != nil {
			return -1, addstate, nil, err
		}
		n.Logger.Trace.Printf("Added block %x\n", block.Hash)
	} else {
		n.Logger.Trace.Printf("Block can not be added. State is %d\n", blockstate)
	}
	return blockstate, addstate, block, nil
}

// Get node state

func (n *Node) GetNodeState() (nodeclient.ComGetNodeState, error) {
	result := nodeclient.ComGetNodeState{}

	result.ExpectingBlocksHeight = 0

	bh, err := n.NodeBC.GetBestHeight()

	if err != nil {
		return result, err
	}
	result.BlocksNumber = bh + 1

	unappr, err := n.GetTransactionsManager().GetUnapprovedCount()

	if err != nil {
		return result, err
	}

	result.TransactionsCached = unappr

	unspent, err := n.GetTransactionsManager().GetUnspentCount()
	if err != nil {
		return result, err
	}

	result.UnspentOutputs = unspent

	return result, nil
}
