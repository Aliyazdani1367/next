package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/nextpanel/next/internal/app/logging"
)

var nextScriptPath = "/usr/local/bin/next"

func runManagedDatabaseMaintenance(ctx context.Context) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("NEXT_INSTALL_MODE")), "binary") {
		return
	}
	if _, err := os.Stat(nextScriptPath); err != nil {
		return
	}

	maintenanceCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	output, err := exec.CommandContext(maintenanceCtx, nextScriptPath, "database-maintenance").CombinedOutput()
	message := strings.TrimSpace(string(output))
	if err != nil {
		logging.Warnf(logging.ComponentRuntime, "managed database maintenance failed: %v output=%s", err, message)
		return
	}
	if message != "" {
		logging.Infof(logging.ComponentRuntime, "managed database maintenance: %s", message)
	}
}
