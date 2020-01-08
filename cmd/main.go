package main

import (
	"os"
	"os/signal"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/phenixrizen/assemblege"
)

func main() {

	log := logrus.New()

	assem, _ := assemblege.NewAssemblage(42280, "eth0", "test", "me", "now.", log)
	err := assem.Run()
	if err != nil {
		log.Fatal(err)
	}

	WaitForCtrlC()
	err = assem.Shutdown()
	if err != nil {
		log.Fatal(err)
	}
}

func WaitForCtrlC() {
	var end_waiter sync.WaitGroup
	end_waiter.Add(1)
	var signal_channel chan os.Signal
	signal_channel = make(chan os.Signal, 1)
	signal.Notify(signal_channel, os.Interrupt)
	go func() {
		<-signal_channel
		end_waiter.Done()
	}()
	end_waiter.Wait()
}
