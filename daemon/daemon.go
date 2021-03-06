// Copyright (c) 2018
// Author: Jeff Weisberg <jaw @ tcp4me.com>
// Created: 2018-Jul-20 22:43 (EDT)
// Function: run as a daemon

// run as a daemon
package daemon

import (
	"fmt"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"
	"time"
)

const (
	ExitFinished = 0
	ExitRestart  = 1
)

const ENVVAR = "_dmode"

var PidfileDir = "/var/run"
var Pidfile = ""

// daemon.Ize() - run program as a daemon
func Ize() {

	mode := os.Getenv(ENVVAR)
	prog, err := os.Executable()

	if err != nil {
		fmt.Printf("cannot daemonize: %v", err)
		os.Exit(2)
	}

	if mode == "" {
		// initial execution
		// switch to the background
		os.Setenv(ENVVAR, "1")
		dn, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0666)
		pa := &os.ProcAttr{Files: []*os.File{dn, dn, dn}}
		os.StartProcess(prog, os.Args, pa)
		os.Exit(0)
	}

	syscall.Setsid()

	if mode == "2" {
		// run and be the main program
		return
	}

	var sigchan = make(chan os.Signal, 5)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)

	if Pidfile == "" && PidfileDir != "" {
		Pidfile = PidfileDir + "/" + path.Base(prog) + ".pid"
	}
	if Pidfile != "" {
		SavePidFile(Pidfile)
	}

	// watch + restart
	for {
		os.Setenv(ENVVAR, "2")
		dn, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0666)
		pa := &os.ProcAttr{Files: []*os.File{dn, dn, dn}}
		p, err := os.StartProcess(prog, os.Args, pa)
		if err != nil {
			fmt.Printf("cannot start %s: %v", prog, err)
			os.Exit(2)
		}

		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			select {
			case <-stop:
				return
			case n := <-sigchan:
				// pass the signal on through to the running program
				p.Signal(n)
			}
		}()

		st, _ := p.Wait()
		if !st.Exited() {
			continue
		}
		if st.Success() {
			// done
			if Pidfile != "" {
				RemovePidFile(Pidfile)
			}
			os.Exit(0)
		}

		close(stop)
		wg.Wait()
		time.Sleep(5 * time.Second)
	}
}

func SavePidFile(file string) error {

	f, err := os.Create(file)
	if err != nil {
		return err
	}

	f.WriteString(fmt.Sprintf("%d\n", os.Getpid()))

	prog, err := os.Executable()
	if err == nil {
		f.WriteString(fmt.Sprintf("# %s", prog))
		for _, arg := range os.Args[1:] {
			f.WriteString(" ")
			f.WriteString(arg)
		}
		f.WriteString("\n")
	}

	f.Close()
	return nil
}

func RemovePidFile(file string) {
	os.Remove(file)
}

func SigExiter() {
	var sigchan = make(chan os.Signal, 5)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)

	select {
	case n := <-sigchan:
		switch n {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			os.Exit(0)
		case syscall.SIGHUP:
			os.Exit(1)
		default:
			os.Exit(2)
		}
	}
}
