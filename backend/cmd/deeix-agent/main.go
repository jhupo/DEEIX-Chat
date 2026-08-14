package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/agentclient"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "install":
		flags := flag.NewFlagSet("install", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		server := flags.String("server", "", "DEEIX server URL")
		user := flags.String("user", "", "DEEIX public user ID")
		workspace := flags.String("workspace", "", "workspace directory")
		name := flags.String("name", hostname(), "device name")
		codex := flags.String("codex", "codex", "Codex CLI executable")
		dataDir := flags.String("data-dir", "", "agent data directory")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *server == "" || *user == "" || *workspace == "" {
			return usageError()
		}
		result, err := agentclient.Install(context.Background(), agentclient.InstallOptions{
			Server: *server, UserPublicID: *user, Workspace: *workspace, Name: *name, CodexExecutable: *codex, DataDir: *dataDir,
		}, os.Stderr)
		if err != nil {
			return err
		}
		return printJSON(result)
	case "start":
		flags := flag.NewFlagSet("start", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", "", "agent data directory")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return usageError()
		}
		if *dataDir == "" {
			var err error
			*dataDir, err = agentclient.DefaultDataDir()
			if err != nil {
				return err
			}
		}
		if err := os.MkdirAll(*dataDir, 0o700); err != nil {
			return err
		}
		logFile, err := os.OpenFile(filepath.Join(*dataDir, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer logFile.Close()
		logger := log.New(logFile, "", log.Ldate|log.Ltime|log.LUTC)
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = agentclient.RunGateway(ctx, *dataDir, logger)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case "service":
		flags := flag.NewFlagSet("service", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", "", "agent data directory")
		userSID := flags.String("user-sid", "", "Windows user SID")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *dataDir == "" || *userSID == "" {
			return usageError()
		}
		return runPlatformService(*dataDir, *userSID)
	case "service-install":
		flags := flag.NewFlagSet("service-install", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", "", "agent data directory")
		userSID := flags.String("user-sid", "", "Windows user SID")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *dataDir == "" || *userSID == "" {
			return usageError()
		}
		return installPlatformService(*dataDir, *userSID)
	case "service-stop":
		if len(args) != 1 {
			return usageError()
		}
		return stopPlatformService()
	case "service-uninstall":
		if len(args) != 1 {
			return usageError()
		}
		return uninstallPlatformService()
	case "doctor":
		flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", "", "agent data directory")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return usageError()
		}
		report, err := agentclient.Doctor(context.Background(), *dataDir, os.Stderr)
		if err != nil {
			return err
		}
		return printJSON(report)
	case "status":
		flags := flag.NewFlagSet("status", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", "", "agent data directory")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return usageError()
		}
		status, err := agentclient.ReadRuntimeStatus(*dataDir)
		if err != nil {
			return err
		}
		return printJSON(status)
	case "update":
		flags := flag.NewFlagSet("update", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", "", "agent data directory")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return usageError()
		}
		result, err := agentclient.Update(context.Background(), *dataDir)
		if err != nil {
			return err
		}
		return printJSON(result)
	case "uninstall":
		flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", "", "agent data directory")
		purge := flags.Bool("purge", false, "delete device identity and local state")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return usageError()
		}
		result, err := agentclient.Uninstall(*dataDir, *purge)
		if err != nil {
			return err
		}
		return printJSON(result)
	case "version", "--version", "-version":
		fmt.Printf("deeix-agent %s (%s) %s\n", version, commit, agentclient.PlatformName())
		return nil
	default:
		return usageError()
	}
}

func usageError() error {
	return errors.New("usage:\n  deeix-agent install --server URL --user PUBLIC_ID --workspace PATH [--name NAME] [--codex PATH]\n  deeix-agent start\n  deeix-agent doctor\n  deeix-agent status\n  deeix-agent update\n  deeix-agent uninstall [--purge]\n  deeix-agent version")
}

func hostname() string {
	value, err := os.Hostname()
	if err != nil || value == "" {
		return runtime.GOOS + "-device"
	}
	return value
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
