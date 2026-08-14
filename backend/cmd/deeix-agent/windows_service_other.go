//go:build !windows

package main

import "errors"

var errWindowsServiceOnly = errors.New("Windows service commands are available only on Windows")

func runPlatformService(string, string) error     { return errWindowsServiceOnly }
func installPlatformService(string, string) error { return errWindowsServiceOnly }
func stopPlatformService() error                  { return errWindowsServiceOnly }
func uninstallPlatformService() error             { return errWindowsServiceOnly }
