// Команда измеряет всплеск авторизованных HTTP-запросов только на локальном тестовом API.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"
)

// sample хранит HTTP-статус и время запроса без тела ответа и токена.
type sample struct {
	status   int
	duration time.Duration
}

// main создаёт тестового пользователя, выполняет 100 GET с конкуренцией 10 и
// печатает задержки и ошибки в JSON. url допускает только loopback, чтобы случайно
// не нагрузить рабочую среду. Тестовый аккаунт остаётся в изолированном стенде.
func main() {
	origin := flag.String("url", "http://127.0.0.1:18085", "isolated API origin")
	flag.Parse()
	parsed, err := url.Parse(*origin)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		fmt.Fprintln(os.Stderr, "loopback test API required")
		os.Exit(2)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	email, password := uuid.NewString()+"@stage6-load.example", uuid.NewString()
	credentials := map[string]string{"email": email, "password": password}
	if status, _, err := request(client, *origin, "POST", "/api/v1/auth/register", "", credentials); err != nil || status != 201 {
		fmt.Fprintln(os.Stderr, "test registration failed")
		os.Exit(1)
	}
	status, data, err := request(client, *origin, "POST", "/api/v1/auth/login", "", credentials)
	var login struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(data, &login)
	if err != nil || status != 200 || login.AccessToken == "" {
		fmt.Fprintln(os.Stderr, "test login failed")
		os.Exit(1)
	}
	results := make(chan sample, 100)
	jobs := make(chan struct{})
	var wg sync.WaitGroup
	started := time.Now()
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				start := time.Now()
				status, _, err := request(client, *origin, "GET", "/api/v1/auth/me", login.AccessToken, nil)
				if err != nil {
					status = 0
				}
				results <- sample{status, time.Since(start)}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()
	close(results)
	latencies := make([]float64, 0, 100)
	statuses := map[int]int{}
	for s := range results {
		latencies = append(latencies, float64(s.duration.Microseconds())/1000)
		statuses[s.status]++
	}
	sort.Float64s(latencies)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"requests": 100, "concurrency": 10, "elapsed_seconds": time.Since(started).Seconds(), "p50_ms": latencies[49], "p95_ms": latencies[94], "p99_ms": latencies[98], "statuses": statuses})
	if statuses[200] != 100 {
		os.Exit(1)
	}
}

// request отправляет ограниченный JSON-запрос; client задаёт конечный timeout,
// origin/path — адрес, token — невыводимый Bearer, body — необязательная нагрузка.
// Возвращает status, максимум 1 МиБ ответа и сетевую ошибку.
func request(client *http.Client, origin, method, path, token string, body any) (int, []byte, error) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(method, origin+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, data, err
}
