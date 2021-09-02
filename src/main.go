package main

import (
	"context"
	"fmt"

	"github.com/google/go-github/v38/github"
)

func main() {
	client := github.NewClient(nil)

	// list all organizations for user "willnorris"
	orgs, _, err := client.Repositories.List(context.Background(), "poorlydefinedbehaviour", nil)
	if err != nil {
		panic(err)
	}

	fmt.Printf("\n\naaaaaaa orgs %+v\n\n", orgs)
}
