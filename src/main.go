package main

import (
	"context"
	"flag"

	"fmt"

	"github.com/google/go-github/v38/github"
)

func main() {
	token := flag.String("token", "", "github access token")

	flag.Parse()

	if *token == "" {
		fmt.Println("github access token is required to make pull requests")
		return
	}

	client := github.NewClient(nil)

	// list all organizations for user "willnorris"
	orgs, _, err := client.Repositories.List(context.Background(), "poorlydefinedbehaviour", nil)
	if err != nil {
		panic(err)
	}

	fmt.Printf("\n\naaaaaaa orgs %+v\n\n", orgs)
}
