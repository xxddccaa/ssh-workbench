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

var version = "0.4.1"

func main() {
	logging.Install(os.Stderr)
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	reportReady, isDaemonChild := initDaemonChild()
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
	if len(arguments) > 0 && arguments[0] == "status" {
		return statusCommand(arguments[1:], defaultDataDirectory)
	}
	if len(arguments) > 0 && arguments[0] == "stop" {
		return stopCommand(arguments[1:], defaultDataDirectory)
	}
	if len(arguments) > 0 && (arguments[0] == "open" || arguments[0] == "edit") {
		return controlCommand(arguments[0], arguments[1:], defaultDataDirectory)
	}
	if len(arguments) > 0 && arguments[0] == "serve" {
		arguments = arguments[1:]
	}
	flags := flag.NewFlagSet("sshw", flag.ContinueOnError)
	var config bootstrap.Config
	var noBrowser bool
	var foreground bool
	flags.StringVar(&config.Listen, "listen", "127.0.0.1:8622", "HTTP listen address")
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
	flags.BoolVar(&foreground, "foreground", false, "run server in the foreground instead of detaching")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}
	config.Version = version
	if err := config.Validate(); err != nil {
		return err
	}

	if !foreground && !isDaemonChild {
		return launchBackground(config, arguments, noBrowser)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	err = bootstrap.Run(ctx, config, func(ready bootstrap.Ready) {
		if ready.RequestedListen != "" && ready.RequestedListen != ready.Listen {
			fmt.Printf("原监听地址 %s 已被其他程序占用，已自动切换至 %s\n", ready.RequestedListen, ready.Listen)
		}
		fmt.Printf("SSH Workbench %s\nListen: %s\nOpen: %s\n", version, ready.Origin, ready.URL)
		if ready.AuthRequired && !ready.Remote {
			fmt.Println("本机授权链接两分钟内有效，仅可使用一次。请勿分享。")
		}
		if isDaemonChild {
			reportReady(daemonMessage{Ready: &ready})
			return
		}
		if !noBrowser {
			if openErr := platform.OpenBrowser(ready.URL); openErr != nil {
				slog.Warn("could not open browser; use the printed URL")
			}
		}
	})
	if errors.Is(err, bootstrap.ErrAlreadyRunning) {
		if isDaemonChild {
			reportReady(daemonMessage{AlreadyRunning: true})
			return nil
		}
		fmt.Println("此数据目录已有服务在运行，改为申请新的授权窗口。")
		return openExistingFromControl(config.DataDir, noBrowser)
	}
	var existing *bootstrap.ExistingServerError
	if errors.As(err, &existing) {
		ready := bootstrap.Ready{Origin: existing.Origin, URL: existing.URL, Listen: existing.Origin, Version: version}
		if isDaemonChild {
			reportReady(daemonMessage{Existing: &ready})
			return nil
		}
		fmt.Printf("检测到端口 %d 已有 SSH Workbench 在运行，直接复用。\nOpen: %s\n", existing.Port, existing.URL)
		if !noBrowser {
			openBrowserOrPrint(existing.URL)
		}
		return nil
	}
	if err != nil && isDaemonChild {
		reportReady(daemonMessage{Error: err.Error()})
	}
	return err
}

func launchBackground(config bootstrap.Config, arguments []string, noBrowser bool) error {
	if handled, err := tryOpenRunningServer(config.DataDir, noBrowser); handled {
		return err
	}
	message, pid, logPath, err := spawnDaemon(config.DataDir, arguments)
	if err != nil {
		return err
	}
	if message.Error != "" {
		return errors.New(message.Error)
	}
	if message.AlreadyRunning {
		return openExistingWithRetry(config.DataDir, noBrowser)
	}
	if message.Existing != nil {
		fmt.Printf("检测到目标端口已有 SSH Workbench 在运行，直接复用。\nOpen: %s\n", message.Existing.URL)
		if !noBrowser {
			openBrowserOrPrint(message.Existing.URL)
		}
		return nil
	}
	if message.Ready == nil {
		return fmt.Errorf("后台服务未返回就绪信息，请查看日志：%s", logPath)
	}
	ready := message.Ready
	if ready.RequestedListen != "" && ready.RequestedListen != ready.Listen {
		fmt.Printf("原监听地址 %s 已被其他程序占用，已自动切换至 %s\n", ready.RequestedListen, ready.Listen)
	}
	fmt.Printf("SSH Workbench %s 已在后台运行 (PID %d)\nListen: %s\nOpen: %s\nLog: %s\n", version, pid, ready.Origin, ready.URL, logPath)
	if ready.AuthRequired && !ready.Remote {
		fmt.Println("本机授权链接两分钟内有效，仅可使用一次。请勿分享。")
	}
	if !noBrowser {
		openBrowserOrPrint(ready.URL)
	}
	return nil
}

func tryOpenRunningServer(directory string, noBrowser bool) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	connection, err := platform.DialControl(ctx, directory)
	if err != nil {
		return false, nil
	}
	response, err := control.Call(connection, control.Request{Command: "open"})
	if err == nil {
		if response.PID > 0 {
			fmt.Printf("SSH Workbench 已在后台运行 (PID %d)\n", response.PID)
		} else {
			fmt.Println("SSH Workbench 已在后台运行。")
		}
		if response.URL != "" {
			if !isLocalControlURL(response.URL) {
				return true, fmt.Errorf("控制通道返回了非本机地址，已拒绝打开")
			}
			fmt.Printf("Open: %s\n", response.URL)
			if strings.Contains(response.URL, "#bootstrap=") {
				fmt.Println("一次性链接两分钟内有效，请勿分享。")
			}
			if !noBrowser {
				openBrowserOrPrint(response.URL)
			}
		}
		return true, nil
	}
	// If the running server is in remote mode, query status instead of `open`.
	statusConn, dialErr := platform.DialControl(ctx, directory)
	if dialErr != nil {
		return true, err
	}
	statusResp, statusErr := control.Call(statusConn, control.Request{Command: "status"})
	if statusErr != nil || !statusResp.Remote {
		return true, err
	}
	if statusResp.PID > 0 {
		fmt.Printf("SSH Workbench 已在后台运行 (PID %d，远程模式)\n", statusResp.PID)
	} else {
		fmt.Println("SSH Workbench 已在后台运行（远程模式）。")
	}
	fmt.Printf("Open: %s\n", statusResp.Origin)
	if !noBrowser && (strings.HasPrefix(statusResp.Origin, "https://") || isLocalControlURL(statusResp.Origin)) {
		openBrowserOrPrint(statusResp.Origin)
	}
	return true, nil
}

func openExistingWithRetry(directory string, noBrowser bool) error {
	deadline := time.Now().Add(2 * time.Second)
	for {
		handled, err := tryOpenRunningServer(directory, noBrowser)
		if handled {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("此数据目录已有服务在运行，但暂时无法连接控制通道")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func openExistingFromControl(directory string, noBrowser bool) error {
	handled, err := tryOpenRunningServer(directory, noBrowser)
	if handled {
		return err
	}
	return fmt.Errorf("没有找到正在运行的本机服务，请先运行 sshw")
}

func openBrowserOrPrint(address string) {
	if err := platform.OpenBrowser(address); err != nil {
		slog.Warn("could not open browser; use the printed URL")
	}
}

func isLocalControlURL(raw string) bool {
	if strings.ContainsAny(raw, " \t\r\n\"") {
		return false
	}
	return strings.HasPrefix(raw, "http://127.0.0.1:") || strings.HasPrefix(raw, "http://[::1]:")
}

func statusCommand(arguments []string, defaultDirectory string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	directory := flags.String("data-dir", defaultDirectory, "private application data directory")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := platform.DialControl(ctx, *directory)
	if err != nil {
		return fmt.Errorf("SSH Workbench 未在后台运行")
	}
	response, err := control.Call(connection, control.Request{Command: "status"})
	if err != nil {
		return err
	}
	ver := response.Version
	if ver == "" {
		ver = version
	}
	fmt.Printf("SSH Workbench %s 正在后台运行\n", ver)
	if response.PID > 0 {
		fmt.Printf("PID: %d\n", response.PID)
	}
	if response.Listen != "" {
		fmt.Printf("Listen: %s\n", response.Listen)
	}
	if response.Origin != "" {
		fmt.Printf("Origin: %s\n", response.Origin)
	}
	if response.Remote {
		fmt.Println("Mode: remote")
	}
	fmt.Printf("Log: %s\n", filepath.Join(*directory, "sshw.log"))
	return nil
}

func stopCommand(arguments []string, defaultDirectory string) error {
	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	directory := flags.String("data-dir", defaultDirectory, "private application data directory")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := platform.DialControl(ctx, *directory)
	if err != nil {
		fmt.Println("SSH Workbench 当前未在后台运行。")
		return nil
	}
	response, err := control.Call(connection, control.Request{Command: "stop"})
	if err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		probe, dialErr := platform.DialControl(probeCtx, *directory)
		probeCancel()
		if dialErr != nil {
			break
		}
		probe.Close()
		time.Sleep(50 * time.Millisecond)
	}
	if response.PID > 0 {
		fmt.Printf("SSH Workbench 已停止 (PID %d)。\n", response.PID)
	} else {
		fmt.Println("SSH Workbench 已停止。")
	}
	return nil
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
		return fmt.Errorf("usage: sshw open | sshw edit FILE")
	}
	return requestControl(command, filename, *directory, *noBrowser)
}

func requestControl(command, filename, directory string, noBrowser bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := platform.DialControl(ctx, directory)
	if err != nil {
		return fmt.Errorf("没有找到正在运行的本机服务，请先运行 sshw")
	}
	response, err := control.Call(connection, control.Request{Command: command, Path: filename})
	if err != nil {
		return err
	}
	// Only ever hand a local server URL to the OS opener.
	if !isLocalControlURL(response.URL) {
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
