package main

import (
	"context"

	"github.com/PoorlyDefinedBehaviour/apply_codemod/src/apply"
)

func main() {
	err := apply.Apply(context.Background(), []apply.Codemod{})
	if err != nil {
		panic(err)
	}
}
