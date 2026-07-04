package main

import "os"

var version = "dev"

func main() {
	os.Args = append([]string{os.Args[0]}, normalizeProfileArgs(os.Args[1:])...)
	if err := NewCommand().Execute(); err != nil {
		failf("%s", err)
	}
}
