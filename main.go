package main


import (
	"context"
	"fmt"
    "github.com/taincoin/taincoin/config"
	"github.com/libp2p/go-libp2p"
    "github.com/common-nighthawk/go-figure"
)

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

