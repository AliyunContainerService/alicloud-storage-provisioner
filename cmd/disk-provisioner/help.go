package main

import "fmt"

var (
	// VERSION should be updated by hand at each release
	VERSION = "v1.10.4"

	// GITCOMMIT will be overwritten automatically by the build system
	GITCOMMIT = "HEAD"
)

func ProvisionVersion() string {
	return VERSION
}

func Usage() {
	fmt.Printf("In K8s Mode: " +
		"--v=5 \n" +
		"Use binary file as the first parameter\n")
}
