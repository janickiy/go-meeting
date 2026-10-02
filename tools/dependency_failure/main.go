// Команда проверяет отказ зависимостей только на изолированном stage6 стенде.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// result содержит измеренные сроки отказа/восстановления одной тестовой службы.
type result struct {
	Service                         string `json:"service"`
	UnreadySeconds, RecoverySeconds float64
	Error                           string `json:"error,omitempty"`
}

// main проверяет точный Compose project label до любой остановки контейнера.
// Аргументов нет; результат — JSON отчёт, ненулевой код при провале проверки.
func main() {
	if os.Getenv("RECORDER_STAGE6_FAILURE") != "true" {
		fmt.Fprintln(os.Stderr, "set RECORDER_STAGE6_FAILURE=true for isolated failure injection")
		os.Exit(2)
	}
	services := []string{"redis", "rabbitmq", "postgres", "minio", "media-worker", "worker"}
	for _, service := range services {
		output, err := command("inspect", "recorder-stage6-"+service+"-1", "--format", "{{index .Config.Labels \"com.docker.compose.project\"}}")
		if err != nil || string(output) != "recorder-stage6\n" {
			fmt.Fprintln(os.Stderr, "isolated project label mismatch")
			os.Exit(2)
		}
	}
	results := make([]result, 0, len(services))
	failed := false
	for _, service := range services {
		target := 18085
		if service == "media-worker" {
			target = 18091
		}
		if service == "worker" {
			target = 18090
		}
		if err := waitStatus(target, 200, 20*time.Second); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		row := result{Service: service}
		started := time.Now()
		if _, err := command("stop", "--time", "25", "recorder-stage6-"+service+"-1"); err != nil {
			row.Error = "stop failed"
		}
		if row.Error == "" {
			if err := waitStatus(target, 503, 20*time.Second); err != nil {
				row.Error = "readiness did not fail"
			}
		}
		row.UnreadySeconds = time.Since(started).Seconds()
		started = time.Now()
		_, startErr := command("start", "recorder-stage6-"+service+"-1")
		if startErr != nil {
			row.Error = "restart failed"
		} else if err := waitStatus(target, 200, 45*time.Second); err != nil {
			row.Error = "readiness did not recover"
		}
		row.RecoverySeconds = time.Since(started).Seconds()
		if row.Error != "" {
			failed = true
		}
		results = append(results, row)
	}
	_ = json.NewEncoder(os.Stdout).Encode(results)
	if failed {
		os.Exit(1)
	}
}

// command запускает docker с отдельными args без shell, ограничивая ожидание
// одной минутой. Возвращает диагностический вывод и ошибку процесса.
func command(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput()
}

// waitStatus ожидает readiness code на локальном port, не более timeout.
// Недоступный процесс приравнивается к 503 только в ожидании его остановки.
func waitStatus(port, code int, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health/ready", port))
		status := 503
		if err == nil {
			status = response.StatusCode
			_ = response.Body.Close()
		}
		if status == code {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("readiness port %d expected %d", port, code)
}
