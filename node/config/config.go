package config

// This code reads command line arguments and config file
import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/taincoin/taincoin/lib/net"
	"github.com/taincoin/taincoin/node/database"
)

// Thi is the struct with all possible command line arguments
type AllPossibleArgs struct {
	Address     string
	From        string
	To          string
	Port        int
	Host        string
	NodePort    int
	NodeHost    string
	Genesis     string
	Amount      float64
	LogDest     string
	Transaction string
	View        string
	Clean       bool
}

// Input summary
type AppInput struct {
	Command       string
	MinterAddress string
	Logs          string
	Port          int
	Host          string
	DataDir       string
	Nodes         []net.NodeAddr
	Args          AllPossibleArgs
	Database      database.DatabaseConfig
}

type AppConfig struct {
	Minter   string
	Port     int
	Host     string
	Nodes    []net.NodeAddr
	Logs     []string
	Database database.DatabaseConfig
}

// Parses inout and config file. Command line arguments ovverride config file options
func GetAppInput() (AppInput, error) {
	input := AppInput{}

	if len(os.Args) < 2 {
		input.Command = "help"
		return input, nil
	}

	input.Command = os.Args[1]

	cmd := flag.NewFlagSet(input.Command, flag.ExitOnError)

	cmd.StringVar(&input.Args.Address, "address", "", "Address of operation")
	cmd.StringVar(&input.Logs, "logs", "", "List of enabled logs groups")
	cmd.StringVar(&input.MinterAddress, "minter", "", "Wallet address which signs blocks")
	cmd.StringVar(&input.Args.Genesis, "genesis", "", "Genesis block text")
	cmd.StringVar(&input.Args.Transaction, "transaction", "", "Transaction ID")
	cmd.StringVar(&input.Args.From, "from", "", "Address to send money from")
	cmd.StringVar(&input.Args.To, "to", "", "Address to send money to")
	cmd.StringVar(&input.Args.Host, "host", "", "Node Server Host")
	cmd.StringVar(&input.Args.NodeHost, "nodehost", "", "Remote Node Server Host")
	cmd.IntVar(&input.Args.Port, "port", 0, "Node Server port")
	cmd.IntVar(&input.Args.NodePort, "nodeport", 0, "Remote Node Server port")
	cmd.Float64Var(&input.Args.Amount, "amount", 0, "Amount money to send")
	cmd.StringVar(&input.Args.LogDest, "logdest", "file", "Destination of logs. file or stdout")
	cmd.StringVar(&input.Args.View, "view", "", "View format")
	cmd.BoolVar(&input.Args.Clean, "clean", false, "Clean data/cache")

	datadirPtr := cmd.String("datadir", "", "Location of data files, config, DB etc")
	err := cmd.Parse(os.Args[2:])

	if err != nil {
		return input, err
	}

	if *datadirPtr != "" {
		input.DataDir = *datadirPtr
		if input.DataDir[len(input.DataDir)-1:] != "/" {
			input.DataDir += "/"
		}
	}
	if input.DataDir == "" {
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))

		if err == nil {
			input.DataDir = dir + "/data/"
		}

	}

	if _, err := os.Stat(input.DataDir); os.IsNotExist(err) {
		os.Mkdir(input.DataDir, 0755)
	}

	input.Port = input.Args.Port
	input.Host = input.Args.Host

	// read config file . command line arguments are more important than a config
	config, err := input.GetConfig()

	if err != nil {
		return input, err
	}
	if config != nil {

		if input.MinterAddress == "" && config.Minter != "" {
			input.MinterAddress = config.Minter
		}

		if input.Port < 1 && config.Port > 0 {
			input.Port = config.Port
		}

		if input.Host == "" && config.Host != "" {
			input.Host = config.Host
		}

		if len(config.Nodes) > 0 {
			input.Nodes = config.Nodes
		}

		if input.Logs == "" && len(config.Logs) > 0 {
			input.Logs = strings.Join(config.Logs, ",")
		}

		input.Database = config.Database
	} else {
		input.Database.SetDefault()
	}
	input.Database.DataDir = input.DataDir

	if input.Host == "" {
		input.Host = "localhost"
	}

	return input, nil
}
func (c AppInput) GetConfig() (*AppConfig, error) {
	file, errf := os.Open(c.DataDir + "config.json")

	if errf != nil && !os.IsNotExist(errf) {
		// error is bad only if file exists but we can not open to read
		return nil, errf
	}
	if errf != nil {
		// config file not found

		return nil, nil
	}

	config := AppConfig{}
	// we open a file only if it exists. in other case options can be set with command line
	decoder := json.NewDecoder(file)
	err := decoder.Decode(&config)

	if err != nil {
		return nil, err
	}

	if config.Database.IsEmpty() {

		config.Database.SetDefault()
	}

	return &config, nil
}
func (c AppInput) CheckNeedsHelp() bool {
	if c.Command == "help" || c.Command == "" {
		return true
	}
	return false
}

func (c AppInput) CheckConfigUpdateNeeded() bool {
	if c.Command == "updateconfig" {
		return true
	}
	return false
}

func (c AppInput) UpdateConfig() error {

	config, err := c.GetConfig()

	if err != nil {
		return err
	}

	if config == nil {
		config = &AppConfig{}
	}

	configfile := c.DataDir + "config.json"

	if c.MinterAddress != "" {
		config.Minter = c.MinterAddress
	}
	if c.Host != "" {
		config.Host = c.Host
	}
	if c.Port > 0 {
		config.Port = c.Port
	}

	if c.Args.NodeHost != "" && c.Args.NodePort > 0 {
		node := net.NodeAddr{c.Args.NodeHost, c.Args.NodePort}

		used := false

		for _, n := range config.Nodes {
			if node.CompareToAddress(n) {
				used = true
				break
			}
		}

		if !used {
			config.Nodes = append(config.Nodes, node)
		}
	}

	if config.Nodes == nil {
		config.Nodes = []net.NodeAddr{}
	}

	if c.Logs != "" {
		if c.Logs == "no" {
			config.Logs = []string{}
		} else {
			config.Logs = strings.Split(c.Logs, ",")
		}

	}

	if config.Logs == nil {
		config.Logs = []string{}
	}

	// convert back to JSON and save to config file
	file, errf := os.OpenFile(configfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)

	if errf != nil {
		return errf
	}

	encoder := json.NewEncoder(file)

	err = encoder.Encode(&config)

	if err != nil {
		return err
	}

	file.Close()
	return nil
}

func (c AppInput) PrintUsage() {
	fmt.Println("Usage:")
	fmt.Println("  help - Prints this help")
	fmt.Println("  == Any of next commands can have optional argument [-datadir /path/to/dir] [-logdest stdout]==")
	fmt.Println("  createwallet\n\t- Generates a new key-pair and saves it into the wallet file")
	fmt.Println("  createblockchain -address ADDRESS -genesis GENESISTEXT\n\t- Create a blockchain and send genesis block reward to ADDRESS")
	fmt.Println("  initblockchain [-nodehost HOST] [-nodeport PORT]\n\t- Loads a blockchain from other node to init the DB.")
	fmt.Println("  printchain [-view short|long]\n\t- Print all the blocks of the blockchain. Default view is long")
	fmt.Println("  makeblock [-minter ADDRESS]\n\t- Try to mine new block if there are enough transactions")
	fmt.Println("  dropblock\n\t- Delete last block fro the block chain. All transaction are returned back to unapproved state")
	fmt.Println("  reindexcache\n\t- Rebuilds the database of unspent transactions outputs and transaction pointers")
	fmt.Println("  showunspent -address ADDRESS\n\t- Print the list of all unspent transactions and balance")
	fmt.Println("  unapprovedtransactions [-clean]\n\t- Print the list of transactions not included in any block yet. If the option -clean provided then cleans the cache")

	fmt.Println("  getbalance -address ADDRESS\n\t- Get balance of ADDRESS")
	fmt.Println("  listaddresses\n\t- Lists all addresses from the wallet file")
	fmt.Println("  getbalances\n\t- Lists all addresses from the wallet file and show balance for each")
	fmt.Println("  addrhistory -address ADDRESS\n\t- Shows all transactions for a wallet address")

	fmt.Println("  send -from FROM -to TO -amount AMOUNT\n\t- Send AMOUNT of coins from FROM address to TO. ")
	fmt.Println("  canceltransaction -transaction TRANSACTIONID\n\t- Cancel unapproved transaction. NOTE!. This cancels only from local cache!")

	fmt.Println("  startnode [-minter ADDRESS] [-host HOST] [-port PORT]\n\t- Start a node server. -minter defines minting address, -host - hostname of the node server and -port - listening port")
	fmt.Println("  startintnode [-minter ADDRESS] [-port PORT]\n\t- Start a node server in interactive mode (no deamon). -minter defines minting address and -port - listening port")
	fmt.Println("  stopnode\n\t- Stop runnning node")
	fmt.Println("  nodestate\n\t- Print state of the node process")
	fmt.Println("  updateconfig [-minter ADDRESS] [-host HOST] [-port PORT] [-nodehost HOST] [-nodeport PORT]\n\t- Update config file. Allows to set this node minter address, host and port and remote node host and port")

	fmt.Println("  shownodes\n\t- Display list of nodes addresses, including inactive")
	fmt.Println("  addnode -nodehost HOST -nodeport PORT\n\t- Adds new node to list of connections")
	fmt.Println("  removenode -nodehost HOST -nodeport PORT\n\t- Removes a node from list of connections")
}
