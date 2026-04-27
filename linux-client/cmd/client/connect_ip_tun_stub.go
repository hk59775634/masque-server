//go:build !linux

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
)

func cmdConnectIPTun(args []string) {
	fs := flag.NewFlagSet("connect-ip-tun", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: client connect-ip-tun (Linux only)\n")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	fmt.Fprintf(os.Stderr, "connect-ip-tun is only supported on Linux (this build is GOOS=%s)\n", runtime.GOOS)
	fmt.Fprintf(os.Stderr, "  On a Linux host: go build -o client ./cmd/client\n")
	os.Exit(1)
}
