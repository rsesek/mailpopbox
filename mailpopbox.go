package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s config.json\n", os.Args[0])
		os.Exit(1)
	}

	configFile, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config file: %s\n", err)
		os.Exit(2)
	}

	var config Config
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		fmt.Fprintf(os.Stderr, "config file: %s\n", err)
		os.Exit(3)
	}
	configFile.Close()

	pop3 := runPOP3Server(config)
	smtp := runSMTPServer(config)

	select {
	case err := <-pop3:
		fmt.Println(err)
	case err := <-smtp:
		fmt.Println(err)
	}
}
