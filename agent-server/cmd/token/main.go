package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aruhiremath92/agent-server/internal/auth"
)

func main() {
	clientID := flag.String("client", "", "dedicated client identifier")
	householdID := flag.String("household", "", "household ID this deployment serves")
	lifetime := flag.Duration("ttl", 24*time.Hour, "token lifetime (maximum 720h)")
	configPath := flag.String("config", "", "root-only server environment file (default: use JWT environment variables)")
	flag.Parse()
	if *clientID == "" || *householdID == "" {
		fmt.Fprintln(os.Stderr, "usage: agent-token -client CLIENT_ID -household HOUSEHOLD_ID [-ttl 24h]")
		os.Exit(2)
	}

	var config auth.Config
	var err error
	if *configPath != "" {
		config, err = auth.ConfigFromFile(*configPath)
	} else {
		config, err = auth.ConfigFromEnvironment()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-token configuration error:", err)
		os.Exit(1)
	}
	token, err := auth.IssueToken(config, strings.TrimSpace(*clientID), strings.TrimSpace(*householdID), time.Now(), *lifetime)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agent-token error:", err)
		os.Exit(1)
	}
	fmt.Println(token)
}
