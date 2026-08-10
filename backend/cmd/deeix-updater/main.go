package main

import (
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/update"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := os.Getenv("DEEIX_UPDATER_DEPLOYMENT_DIR")
	cfg := update.HostConfig{Repository: os.Getenv("DEEIX_UPDATER_REPOSITORY"), SocketPath: os.Getenv("DEEIX_UPDATER_SOCKET_PATH"), StateFile: os.Getenv("DEEIX_UPDATER_STATE_FILE"), DeploymentDir: dir, ComposeFile: os.Getenv("DEEIX_UPDATER_COMPOSE_FILE"), EnvFile: os.Getenv("DEEIX_UPDATER_ENV_FILE"), AppBaseURL: os.Getenv("DEEIX_UPDATER_APP_BASE_URL"), PullTimeout: 30 * time.Minute, ReadyTimeout: 5 * time.Minute}
	if cfg.ComposeFile == "" {
		cfg.ComposeFile = filepath.Join(dir, "compose.yaml")
	}
	if cfg.EnvFile == "" {
		cfg.EnvFile = filepath.Join(dir, ".env")
	}
	u, e := update.NewUpdater(cfg)
	if e != nil {
		log.Fatal(e)
	}
	log.Fatal(update.Serve(u, cfg.SocketPath))
}
