package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func installService() {
	if runtime.GOOS != "linux" {
		log.Fatal("Service installation is only supported on Linux.")
	}

	if os.Geteuid() != 0 {
		log.Fatal("Run with sudo to install service.")
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exePath, _ = filepath.Abs(exePath)
	workDir := filepath.Dir(exePath)
	envFile := filepath.Join(workDir, "env")

	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(workDir, ".env")); err == nil {
			envFile = filepath.Join(workDir, ".env")
		}
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=Anthropic to OpenAI Proxy
After=network.target

[Service]
Type=simple
ExecStart=%s
WorkingDirectory=%s
EnvironmentFile=-%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, exePath, workDir, envFile)

	servicePath := "/etc/systemd/system/ant2oa.service"
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		log.Fatalf("Failed to write service file: %v", err)
	}

	log.Printf("Service file written to %s", servicePath)

	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "ant2oa"},
		{"systemctl", "restart", "ant2oa"},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("Command failed: %v", err)
		}
	}

	log.Println("Service installed and started successfully!")
}
