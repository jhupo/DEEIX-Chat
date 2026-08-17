//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/agentclient"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const windowsServiceName = "DEEIXAgent"

type windowsService struct {
	dataDir string
	logger  *log.Logger
}

func runPlatformService(dataDir, userSID string) error {
	dataDir, userSID, err := validateServiceArguments(dataDir, userSID)
	if err != nil {
		return err
	}
	if err = os.Setenv(agentclient.WindowsUserSIDEnvironment, userSID); err != nil {
		return err
	}
	if err = os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	return svc.Run(windowsServiceName, &windowsService{dataDir: dataDir, logger: log.New(logFile, "", log.Ldate|log.Ltime|log.LUTC)})
}

func (service *windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	updateReady := make(chan struct{}, 1)
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			err := runAgentGateway(ctx, service.dataDir, service.logger)
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, agentclient.ErrUpdateScheduled) {
				updateReady <- struct{}{}
				return
			}
			service.logger.Printf("agent runtime stopped: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}()
	statuses <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case <-updateReady:
			statuses <- svc.Status{State: svc.StopPending}
			cancel()
			<-done
			return false, 0
		case request, open := <-requests:
			if !open {
				cancel()
				<-done
				return false, 0
			}
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending}
				cancel()
				<-done
				return false, 0
			}
		}
	}
}

func installPlatformService(dataDir, userSID string) error {
	dataDir, userSID, err := validateServiceArguments(dataDir, userSID)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows service manager: %w", err)
	}
	defer manager.Disconnect()
	if existing, openErr := manager.OpenService(windowsServiceName); openErr == nil {
		if err = stopService(existing, 30*time.Second); err != nil {
			existing.Close()
			return err
		}
		if err = existing.Delete(); err != nil {
			existing.Close()
			return fmt.Errorf("replace Windows service: %w", err)
		}
		existing.Close()
		if err = waitServiceDeletion(manager, 30*time.Second); err != nil {
			return err
		}
	} else if !errors.Is(openErr, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return fmt.Errorf("open Windows service: %w", openErr)
	}
	service, err := manager.CreateService(windowsServiceName, executable, mgr.Config{
		StartType: mgr.StartAutomatic, DisplayName: "DEEIX Agent", Description: "Connects this device to DEEIX and runs the local Codex app-server.", DelayedAutoStart: true,
	}, "service", "--data-dir", dataDir, "--user-sid", userSID)
	if err != nil {
		return fmt.Errorf("install Windows service: %w", err)
	}
	defer service.Close()
	if err = service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 10 * time.Second}}, 86400); err != nil {
		_ = service.Delete()
		return fmt.Errorf("configure Windows service recovery: %w", err)
	}
	if err = service.Start(); err != nil {
		_ = service.Delete()
		return fmt.Errorf("start Windows service: %w", err)
	}
	return nil
}

func stopPlatformService() error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows service manager: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(windowsServiceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open Windows service: %w", err)
	}
	defer service.Close()
	return stopService(service, 30*time.Second)
}

func uninstallPlatformService() error {
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows service manager: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(windowsServiceName)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open Windows service: %w", err)
	}
	defer service.Close()
	if err = stopService(service, 30*time.Second); err != nil {
		return err
	}
	if err = service.Delete(); err != nil {
		return fmt.Errorf("delete Windows service: %w", err)
	}
	return nil
}

func validateServiceArguments(dataDir, userSID string) (string, string, error) {
	dataDir = strings.TrimSpace(dataDir)
	userSID = strings.TrimSpace(userSID)
	if dataDir == "" || strings.ContainsRune(dataDir, 0) {
		return "", "", errors.New("Windows service data directory is invalid")
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return "", "", err
	}
	sid, err := windows.StringToSid(userSID)
	if err != nil || !strings.EqualFold(sid.String(), userSID) {
		return "", "", errors.New("Windows service user SID is invalid")
	}
	return absolute, sid.String(), nil
}

func stopService(service *mgr.Service, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	stopRequested := false
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return fmt.Errorf("query Windows service: %w", err)
		}
		if status.State == svc.Stopped {
			return nil
		}
		if !stopRequested && status.Accepts&svc.AcceptStop != 0 {
			if _, err = service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
				return fmt.Errorf("stop Windows service: %w", err)
			}
			stopRequested = true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("Windows service did not stop in time")
}

func waitServiceDeletion(manager *mgr.Mgr, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		service, err := manager.OpenService(windowsServiceName)
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("wait for Windows service deletion: %w", err)
		}
		service.Close()
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("Windows service was not deleted in time")
}
