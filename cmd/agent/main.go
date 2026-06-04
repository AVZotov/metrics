package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/AVZotov/metrics/internal/agent"
	"github.com/AVZotov/metrics/internal/config"
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.NewAgentConfig()
	if err != nil {
		return err
	}
	client := &http.Client{}
	baseURL := fmt.Sprintf("http://%s", cfg.String())
	a := agent.NewAgent(client, baseURL)

	go func(pi uint) {
		for {
			a.Collect()
			time.Sleep(time.Duration(pi) * time.Second)
		}
	}(cfg.PollInterval)

	go func(ri uint) {
		for {
			if err := a.Report(); err != nil {
				log.Println(err)
			}
			time.Sleep(time.Duration(ri) * time.Second)
		}
	}(cfg.ReportInterval)

	select {}
}
