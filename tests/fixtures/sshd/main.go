package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"sshworkbench/tests/fixtures/sshserver"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	address := flag.String("listen", "127.0.0.1:22222", "loopback-only fixture address")
	echoAddress := flag.String("echo-listen", "", "optional loopback-only TCP echo address for tunnel tests")
	clientKeyPath := flag.String("client-key-out", "", "optional file to receive the fixture client private key (0600)")
	flag.Parse()
	server, err := sshserver.Start(*address)
	if err != nil {
		return err
	}
	defer server.Close()
	var echo *sshserver.Echo
	if *echoAddress != "" {
		echo, err = sshserver.StartEcho(*echoAddress)
		if err != nil {
			return err
		}
		defer echo.Close()
	}
	if *clientKeyPath != "" {
		if err := writePrivateFile(*clientKeyPath, server.ClientKeyPEM()); err != nil {
			return err
		}
		defer os.Remove(*clientKeyPath)
	}

	// Everything is ready before anything is printed, and the banner is a single
	// unbuffered write so a launcher sees all lines together.
	var banner strings.Builder
	fmt.Fprintf(&banner, "Fixture SSH: %s\nUser: %s\nPassword: %s\n", server.Address(), sshserver.Username, sshserver.Password)
	fmt.Fprintf(&banner, "MFA user: %s\nMFA password: %s\nMFA code: %s\n", sshserver.MFAUsername, sshserver.MFAPassword, sshserver.MFACode)
	fmt.Fprintf(&banner, "Key user: %s\n", sshserver.KeyUsername)
	if echo != nil {
		fmt.Fprintf(&banner, "Fixture echo: %s\n", echo.Address())
	}
	if *clientKeyPath != "" {
		fmt.Fprintf(&banner, "Fixture key: %s\n", *clientKeyPath)
	}
	banner.WriteString("In-memory filesystem; no real shell.\n")
	if _, err := os.Stdout.WriteString(banner.String()); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	server.RunUntil(ctx)
	return nil
}

func writePrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// OpenFile keeps the mode of an existing file, so tighten it explicitly.
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
