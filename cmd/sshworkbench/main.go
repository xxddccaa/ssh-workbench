package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"sshworkbench/internal/adapters/platform"
	"sshworkbench/internal/adapters/sqlite"
	"sshworkbench/internal/auth"
	"sshworkbench/internal/bootstrap"
	"sshworkbench/internal/control"
	"sshworkbench/internal/logging"
)

var version = "0.2.0"

func main() {
	logging.Install(os.Stderr)
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	configurationDirectory, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	defaultDataDirectory := filepath.Join(configurationDirectory, "ssh-workbench")
	if len(arguments) > 0 && arguments[0] == "version" {
		fmt.Println(version)
		return nil
	}
	if len(arguments) > 0 && arguments[0] == "upgrade" {
		return upgradeCommand(arguments[1:])
	}
	if len(arguments) > 0 && arguments[0] == "password" {
		return setPassword(arguments[1:], defaultDataDirectory)
	}
	if len(arguments) > 0 && (arguments[0] == "open" || arguments[0] == "edit") {
		return controlCommand(arguments[0], arguments[1:], defaultDataDirectory)
	}
	if len(arguments) > 0 && arguments[0] == "serve" {
		arguments = arguments[1:]
	}
	flags := flag.NewFlagSet("sshworkbench", flag.ContinueOnError)
	var config bootstrap.Config
	var noBrowser bool
	flags.StringVar(&config.Listen, "listen", "127.0.0.1:7331", "HTTP listen address")
	flags.StringVar(&config.DataDir, "data-dir", defaultDataDirectory, "private application data directory")
	flags.BoolVar(&config.Auth, "auth", false, "require bootstrap token or owner password authentication")
	flags.BoolVar(&config.Remote, "remote", false, "enable authenticated HTTPS mode")
	flags.StringVar(&config.Origin, "origin", "", "exact public HTTPS origin in remote mode")
	flags.StringVar(&config.DevOrigin, "dev-origin", "", "explicit loopback Vite origin, local development only")
	flags.StringVar(&config.TLSCert, "tls-cert", "", "TLS certificate PEM")
	flags.StringVar(&config.TLSKey, "tls-key", "", "TLS private key PEM")
	flags.BoolVar(&config.AllowAgent, "allow-agent", false, "explicitly allow server-side SSH agent in remote mode")
	flags.BoolVar(&config.DisableLocalSSH, "no-local-ssh", false, "disable reading ~/.ssh/config, ~/.ssh keys, and SSH agent")
	flags.BoolVar(&noBrowser, "no-browser", false, "do not open the browser")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}
	config.Version = version
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err = bootstrap.Run(ctx, config, func(ready bootstrap.Ready) {
		fmt.Printf("SSH Workbench %s\nListen: %s\nOpen: %s\n", version, ready.Origin, ready.URL)
		if ready.AuthRequired && !ready.Remote {
			fmt.Println("本机授权链接两分钟内有效，仅可使用一次。请勿分享。")
		}
		if !noBrowser {
			if err := platform.OpenBrowser(ready.URL); err != nil {
				slog.Warn("could not open browser; use the printed URL")
			}
		}
	})
	if errors.Is(err, bootstrap.ErrAlreadyRunning) {
		fmt.Println("此数据目录已有服务在运行，改为申请新的授权窗口。")
		return requestControl("open", "", config.DataDir, noBrowser)
	}
	return err
}

// controlCommand talks to the running local server over the OS-user-only
// control channel: `open` issues a fresh one-time browser authorization,
// `edit FILE` registers a local text file and opens it in the editor.
func controlCommand(command string, arguments []string, defaultDirectory string) error {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	directory := flags.String("data-dir", defaultDirectory, "private application data directory")
	noBrowser := flags.Bool("no-browser", false, "print the one-time link instead of opening the browser")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	var filename string
	switch {
	case command == "open" && flags.NArg() == 0:
	case command == "edit" && flags.NArg() == 1:
		absolute, err := filepath.Abs(flags.Arg(0))
		if err != nil {
			return err
		}
		filename = absolute
	default:
		return fmt.Errorf("usage: sshworkbench open | sshworkbench edit FILE")
	}
	return requestControl(command, filename, *directory, *noBrowser)
}

func requestControl(command, filename, directory string, noBrowser bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := platform.DialControl(ctx, directory)
	if err != nil {
		return fmt.Errorf("没有找到正在运行的本机服务，请先运行 sshworkbench")
	}
	response, err := control.Call(connection, control.Request{Command: command, Path: filename})
	if err != nil {
		return err
	}
	// Only ever hand a local server URL to the OS opener.
	if !strings.HasPrefix(response.URL, "http://127.0.0.1:") || strings.ContainsAny(response.URL, " \t\r\n\"") {
		return fmt.Errorf("控制通道返回了非本机地址，已拒绝打开")
	}
	if noBrowser {
		fmt.Println("Open:", response.URL)
		if strings.Contains(response.URL, "#bootstrap=") {
			fmt.Println("一次性链接两分钟内有效，请勿分享。")
		}
		return nil
	}
	if err := platform.OpenBrowser(response.URL); err != nil {
		fmt.Println("Open:", response.URL)
	}
	return nil
}

func setPassword(arguments []string, defaultDirectory string) error {
	flags := flag.NewFlagSet("password", flag.ContinueOnError)
	directory := flags.String("data-dir", defaultDirectory, "private application data directory")
	fromStdin := flags.Bool("stdin", false, "read the password from stdin instead of a hidden prompt")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	var password string
	if *fromStdin {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 1024), 2048)
		if !scanner.Scan() {
			return fmt.Errorf("could not read password: %v", scanner.Err())
		}
		password = scanner.Text()
	} else {
		fmt.Fprint(os.Stderr, "Owner password (12+ characters): ")
		first, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stderr, "Confirm password: ")
		second, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		if string(first) != string(second) {
			return fmt.Errorf("passwords do not match")
		}
		password = string(first)
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("password must not be blank")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	store, err := sqlite.Open(*directory)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.PutSetting(context.Background(), "owner_password", hash); err != nil {
		return err
	}
	fmt.Println("Owner password saved. Restart the server to apply changes and invalidate existing sessions.")
	return nil
}
