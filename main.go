package main

import (
	"apply_codemod/src/apply"
	"context"
)

func main() {
	err := apply.Apply(context.Background(), []apply.Codemod{})
	if err != nil {
		panic(err)
	}
}
