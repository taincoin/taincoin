package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/DavidGamba/go-getoptions"
	"github.com/common-nighthawk/go-figure"
	"github.com/libp2p/go-libp2p"
	"github.com/taincoin/taincoin/config"
)

var logger = log.New(ioutil.Discard, "DEBUG: ", log.LstdFlags)

func initServer() {
	config, err := config.ReadConfig(`config.txt`)
	if err != nil {
		fmt.Println(err)
	}

	// assign values from config file to variables
	ip := config["ip"]
	pass := config["password"]
	port := config["port"]

	fmt.Println("IP :", ip)
	fmt.Println("Port :", port)
	fmt.Println("Password :", pass)
}

func main() {
	var debug bool
	var portNumber int
	var list map[string]string

	opt := getoptions.New()
	opt.Bool("help", false, opt.Alias("h", "?"))
	opt.BoolVar(&debug, "debug", false)
	//opt.Required(),
	opt.IntVar(&portNumber, "port", 8888,
		opt.Description("Number of times to port."))
	opt.StringMapVar(&list, "list", 1, 99,
		opt.Description("Greeting list by language."))
	remaining, err := opt.Parse(os.Args[1:])
	if opt.Called("help") {
		fmt.Fprintf(os.Stderr, opt.Help())
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n\n", err)
		fmt.Fprintf(os.Stderr, opt.Help(getoptions.HelpSynopsis))
		os.Exit(1)
	}

	// Use the passed command line options... Enjoy!
	if debug {
		logger.SetOutput(os.Stderr)
	}
	logger.Printf("Unhandled CLI args: %v\n", remaining)

	// Use the int variable
	fmt.Printf("port number : %d \n", portNumber)

	// Use the map[string]string variable
	if len(list) > 0 {
		fmt.Printf("Greeting List:\n")
		for k, v := range list {
			fmt.Printf("\t%s=%s\n", k, v)
		}
	}

	myFigure := figure.NewFigure("Taincoin", "", true)
	myFigure.Print()
	fmt.Printf("\nWelcome to Toincoin v0.0.1\n\n")

	initServer()

	// create a background context (i.e. one that never cancels)
	ctx := context.Background()

	// start a libp2p node with default settings
	node, err := libp2p.New(ctx)
	if err != nil {
		panic(err)
	}

	// print the node's listening addresses
	fmt.Println("Listen addresses:", node.Addrs())

	// shut the node down
	if err := node.Close(); err != nil {
		panic(err)
	}
}
