package integration_test

import (
	"crypto/tls"
	"encoding/xml"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestRecordingsProxyRoutes проверяет настоящий локальный HTTPS-прокси без авторизации и изменения данных.
// Тест включается только переменной RECORDER_RECORDINGS_PROXY_TEST_URL с loopback-адресом стенда.
// @args t — исполнитель проверки SPA-маршрутов и приватных вложенных путей MinIO.
func TestRecordingsProxyRoutes(t *testing.T) {
	raw := os.Getenv("RECORDER_RECORDINGS_PROXY_TEST_URL")
	if raw == "" {
		t.Skip("укажите RECORDER_RECORDINGS_PROXY_TEST_URL=https://localhost:18482 для локальной проверки прокси")
	}
	base, err := url.Parse(raw)
	if err != nil {
		t.Fatal("некорректный адрес локального прокси")
	}
	host := base.Hostname()
	ip := net.ParseIP(host)
	if base.Scheme != "https" || base.Host == "" ||
		(host != "localhost" && (ip == nil || !ip.IsLoopback())) ||
		base.User != nil || base.RawQuery != "" || base.Fragment != "" ||
		(base.Path != "" && base.Path != "/") {
		t.Fatal("для проверки разрешён только явный HTTPS loopback-origin без credentials и параметров")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	// Самоподписанный сертификат разрешён только этому тесту после строгой проверки loopback-origin.
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		// Не следуем перенаправлению: иначе ошибочный 301 со сброшенным портом останется незамеченным.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}

	const conferenceID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const recordingID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const token = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	spaPaths := []string{
		"/recordings?conference=" + conferenceID,
		"/recordings/?conference=" + conferenceID,
		"/recordings/" + recordingID + "?conference=" + conferenceID,
		"/recordings/" + recordingID + "/?conference=" + conferenceID,
	}
	for _, path := range spaPaths {
		t.Run("SPA"+strings.Split(path, "?")[0], func(t *testing.T) {
			response, body := recordingsProxyGet(t, client, base, path)
			if response.StatusCode != http.StatusOK || response.Header.Get("Location") != "" {
				t.Fatalf("SPA должен отвечать 200 без перенаправления: HTTP %d", response.StatusCode)
			}
			contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
			if err != nil || contentType != "text/html" {
				t.Fatal("SPA не вернул HTML-документ")
			}
			if !strings.Contains(strings.ToLower(string(body)), "<html") ||
				!strings.Contains(string(body), `id="root"`) {
				t.Fatal("ответ не является index.html приложения")
			}
		})
	}

	storagePaths := []string{
		"/recordings/records/" + recordingID + "/final.mp4",
		"/recordings/recordings/" + conferenceID + "/" + recordingID + "/artifacts/" + token + "/final.mp4",
		"/recordings/recordings/" + conferenceID + "/" + recordingID + "/artifacts/" + token + "/preview.jpg",
		"/recordings/recordings/" + conferenceID + "/" + recordingID + "/artifacts/" + token + "/tracks.zip",
		"/recordings/" + recordingID + "/artifacts/" + token + "/final.mp4",
	}
	for _, path := range storagePaths {
		t.Run("MinIO"+path, func(t *testing.T) {
			response, body := recordingsProxyGet(t, client, base, path)
			if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusNotFound {
				t.Fatalf("приватный путь без подписи должен отвечать 403/404: HTTP %d", response.StatusCode)
			}
			if response.Header.Get("Location") != "" {
				t.Fatal("путь артефакта не должен перенаправляться или менять подписываемый адрес")
			}
			contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
			if err != nil || (contentType != "application/xml" && contentType != "text/xml") {
				t.Fatal("путь артефакта не вернул XML-ответ MinIO")
			}
			var storageError struct {
				XMLName xml.Name
				Code    string `xml:"Code"`
			}
			if err := xml.Unmarshal(body, &storageError); err != nil ||
				storageError.XMLName.Local != "Error" || storageError.Code == "" {
				t.Fatal("ответ не является ошибкой S3/MinIO")
			}
			if strings.Contains(string(body), `id="root"`) {
				t.Fatal("приватный файл ошибочно заменён SPA-документом")
			}
		})
	}
}

// recordingsProxyGet делает единственный GET и ограничивает чтение ответа одним МиБ.
// @args t — исполнитель; client — клиент без переходов; base — проверенный loopback-origin; path — проверяемый путь без секретов.
// @return HTTP-ответ без открытого тела и его ограниченные байты.
func recordingsProxyGet(t *testing.T, client *http.Client, base *url.URL, path string) (*http.Response, []byte) {
	t.Helper()
	relative, err := url.Parse(path)
	if err != nil {
		t.Fatal("некорректный тестовый путь")
	}
	target := base.ResolveReference(relative)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		t.Fatal("не удалось подготовить GET к локальному прокси")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("не удалось получить ответ локального HTTPS-прокси")
	}
	defer response.Body.Close()
	const limit = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(body) > limit {
		t.Fatal("не удалось прочитать ограниченный ответ прокси")
	}
	return response, body
}
