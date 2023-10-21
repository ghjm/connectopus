package exit_handler

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// The exit_handler package provides a central signal handler and registration system for adding handlers that
// should run at program shutdown.  To use this, call RunExitFuncs() at the end of your main(), and also anywhere
// you call os.Exit().  Then, use AddExitFunc() to add individual functions that should run at exit.

type exitFunc struct {
	f    func()
	once sync.Once
}

var exitFuncs []*exitFunc

// AddExitFunc adds a given function to the list of functions that should return at exit.  Its return value is
// the same function, wrapped in a sync.Once.  If for some reason you want to call the handler before program
// exit, you can call the returned function.
func AddExitFunc(f func()) func() {
	ef := exitFunc{
		once: sync.Once{},
	}
	ef.f = func() {
		ef.once.Do(f)
	}
	exitFuncs = append(exitFuncs, &ef)
	return ef.f
}

// RunExitFuncs runs all the registered exit funcs.  This function should be called at the end of main(),
// or whenever the program is calling os.Exit().
func RunExitFuncs() {
	for _, ef := range exitFuncs {
		ef.f()
	}
}

func init() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigs
		RunExitFuncs()
		if s == syscall.SIGINT {
			os.Exit(130)
		}
		if s == syscall.SIGTERM {
			os.Exit(143)
		}
		os.Exit(255)
	}()
}
