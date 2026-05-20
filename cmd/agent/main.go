package main

import (
	"log"
	"net/http"
	"time"

	"github.com/AVZotov/metrics/internal/agent"
)

const (
	pollInterval   = 2
	reportInterval = 10
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	client := &http.Client{}
	baseURL := "http://localhost:8080"
	a := agent.NewAgent(client, baseURL)

	go func() {
		for {
			a.Collect()
			time.Sleep(time.Duration(pollInterval) * time.Second)
		}
	}()

	go func() {
		for {
			if err := a.Report(); err != nil {
				log.Println(err)
			}
			time.Sleep(time.Duration(reportInterval) * time.Second)
		}
	}()

	select {}
}
