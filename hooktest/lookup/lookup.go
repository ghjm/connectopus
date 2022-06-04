package main

import (
	"fmt"
	"net"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: lookup <hostname>")
		os.Exit(1)
	}
	addrs, err := net.LookupHost(os.Args[1])
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Results: %v\n", addrs)
}
