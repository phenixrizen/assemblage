package main

import (
	"os"
	"os/signal"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/phenixrizen/assemblage"
)

func main() {

	log := logrus.New()

	assem, _ := assemblage.NewAssemblage(42280, "eth0", "test", "me", "now.", log)
	err := assem.Run()
	if err != nil {
		log.Fatal(err)
	}

	r := gin.Default()
	r.GET("/lookup/:hash", func(c *gin.Context) {
		hash := c.Param("hash")
		nodes := assem.GetNodes(hash, 1)
		c.JSON(200, gin.H{
			"hash":  hash,
			"nodes": nodes,
		})
	})
	go r.Run("0.0.0.0:42280")

	waitForCtrlC()
	err = assem.Shutdown()
	if err != nil {
		log.Fatal(err)
	}
}

func waitForCtrlC() {
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
