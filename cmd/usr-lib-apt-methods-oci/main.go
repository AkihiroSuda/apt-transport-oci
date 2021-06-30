package main

import (
	"context"
	"os"

	"github.com/AkihiroSuda/apt-transport-oci/pkg/method"
)

func main() {
	method.New(os.Stdout, os.Stdin).Run(context.Background())
}
