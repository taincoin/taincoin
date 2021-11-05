package main

import (
	"fmt"
	"os"

	"github.com/common-nighthawk/go-figure"
	"github.com/taincoin/taincoin/lib"
	"github.com/taincoin/taincoin/log"
	"github.com/taincoin/taincoin/node/config"
)

// all loggers can have key/value context
var srvlog = log.New("module", "app/server")
var connlog log.Logger

func initServer() {
	// init Log
	srvlog.SetHandler(log.MultiHandler(
		log.StreamHandler(os.Stderr, log.LogfmtFormat()),
		log.LvlFilterHandler(
			log.LvlError,
			log.Must.FileHandler("errors.json", log.JSONFormat()))))

	myFigure := figure.NewFigure("Taincoin", "", true)
	myFigure.Print()
	fmt.Printf("\nWelcome to Toincoin v0.0.1\n\n")
}

func main() {
	// Parse input
	input, ierr := config.GetAppInput()

	initServer()

	if ierr != nil {
		// something went wrong when parsing input data
		//fmt.Printf("Error: %s\n", ierr.Error())
		srvlog.Warn("something went wrong when parsing input data", "Error", ierr.Error())

		os.Exit(0)
	}

	if input.CheckNeedsHelp() {
		//fmt.Printf("%s - %s\n\n", lib.ApplicationTitle, lib.ApplicationVersion)
		srvlog.Warn("CheckNeedsHelp", "ApplicationTitle", lib.ApplicationTitle, "ApplicationVersion", lib.ApplicationVersion)

		// if user requested a help, display it
		input.PrintUsage()
		os.Exit(0)
	}

	if input.CheckConfigUpdateNeeded() {
		//fmt.Printf("%s - %s\n\n", lib.ApplicationTitle, lib.ApplicationVersion)
		srvlog.Warn("CheckConfigUpdateNeeded", "ApplicationTitle", lib.ApplicationTitle, "ApplicationVersion", lib.ApplicationVersion)

		// save config using input arguments
		input.UpdateConfig()
		os.Exit(0)
	}
	// create node client object
	// this will create all other objects needed to execute a command
	cli := getNodeCLI(input)

	if cli.isInteractiveMode() {
		//fmt.Printf("%s - %s\n\n", lib.ApplicationTitle, lib.ApplicationVersion)
		srvlog.Warn("isInteractiveMode", "ApplicationTitle", lib.ApplicationTitle, "ApplicationVersion", lib.ApplicationVersion)

		// it is command to display results right now
		err := cli.ExecuteCommand()

		if err != nil {
			//fmt.Printf("Error: %s\n", err.Error())
			srvlog.Warn("ExecuteCommand", "Error", err.Error())
		}
		os.Exit(0)
	}

	if cli.isNodeManageMode() {
		// it is the command to manage node server
		err := cli.ExecuteManageCommand()

		if err != nil {
			//fmt.Printf("Node Manage Error: %s\n", err.Error())
			srvlog.Warn("Node Manage", "Error", err.Error())

		}

		os.Exit(0)
	}

	fmt.Println("Unknown command!")
	input.PrintUsage()
}
