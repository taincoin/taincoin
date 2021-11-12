package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	daemon   bool
	fork     int
	children = []int{}
)

func init() {
	flag.BoolVar(&daemon, "daemon", false, "run as daemon")
	flag.IntVar(&fork, "fork", 0, "fork how many sub process")

	flag.Parse()
}

func main() {
	pid := os.Getpid()
	ppid := os.Getppid()
	log.Printf("pid: %d, ppid: %d, args: %s", pid, ppid, os.Args)

	// handle exit for every process
	go func() {
		sig := make(chan os.Signal)
		signal.Notify(sig)
		for s := range sig {
			// see https://u.kfd.me/33
			// SIGINT means graceful stop
			// SIGTERM means graceful [or not], cleanup something
			// SIGQUIT SIGKILL means immediately shutdown
			if s == syscall.SIGQUIT || s == syscall.SIGTERM {
				log.Printf("[%d] exit\n", pid)
				// make sure that parent can send signals to the children
				for _, child := range children {
					log.Printf("parent send %s to %d", s, child)
					syscall.Kill(child, s.(syscall.Signal))
				}
				syscall.Exit(0)
			}
		}
	}()

	// only the parent process can do
	if _, isChild := os.LookupEnv("CHILD_ID"); !isChild {
		if daemon {
			if _, isDaemon := os.LookupEnv("DAEMON"); !isDaemon {
				daemonENV := []string{"DAEMON=true"}
				childPID, _ := syscall.ForkExec(os.Args[0], os.Args,
					&syscall.ProcAttr{
						Env: append(os.Environ(), daemonENV...),
						Sys: &syscall.SysProcAttr{
							Setsid: true,
						},
						Files: []uintptr{0, 1, 2}, // print message to the same pty
					})
				log.Printf("process %d run as daemon, %d will exit", childPID, pid)
				return // this return will give back the pty
			}
			log.Printf("daemon %d running and won't fork another daemon", os.Getpid())
		}

		for i := 0; i < fork; i++ {
			args := append(os.Args, fmt.Sprintf("#child_%d_of_%d", i, os.Getpid()))
			childENV := []string{
				fmt.Sprintf("CHILD_ID=%d", i),
			}
			pwd, err := os.Getwd()
			if err != nil {
				log.Fatalf("getwd err: %s", err)
			}
			childPID, _ := syscall.ForkExec(args[0], args, &syscall.ProcAttr{
				Dir: pwd,
				Env: append(os.Environ(), childENV...),
				Sys: &syscall.SysProcAttr{
					Setsid: true,
				},
				Files: []uintptr{0, 1, 2}, // print message to the same pty
			})
			log.Printf("parent %d fork %d", pid, childPID)
			if childPID != 0 {
				children = append(children, childPID)
			}
		}
		// print children
		log.Printf("parent: PID=%d children=%v", pid, children)
		if len(children) == 0 && fork != 0 {
			log.Fatalf("no child avaliable, exit")
		}

		// set env
		for _, childID := range children {
			if c := os.Getenv("CHILDREN"); c != "" {
				os.Setenv("CHILDREN", fmt.Sprintf("%s,%d", c, childID))
			} else {
				os.Setenv("CHILDREN", fmt.Sprintf("%d", childID))
			}
		}

		go func() {
			// parent will [only] notify children
			sig := make(chan os.Signal)
			signal.Notify(sig)
			for s := range sig {
				for _, child := range children {
					log.Printf("parent send %s to %d", s, child)
					syscall.Kill(child, s.(syscall.Signal))
				}
			}
		}()

	}

	// main
	sig := make(chan os.Signal)
	signal.Notify(sig)
	for s := range sig {
		log.Printf("PID[%d] got sig %s", pid, s)
	}
}
