package contentproviders

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	domain "github.com/janickiy/go-recorder/internal/domain/search"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// embeddings использует общий защищённый HTTP клиент без redirects и вывода ответа в логи.
type embeddings struct{ *provider }

// NewEmbeddingProvider создаёт noop/mock/http адаптер без вызова платного сервиса.
// @args mode,endpoint,token — операторские настройки; timeout — конечный deadline.
// @return интерфейс провайдера либо ошибка конфигурации.
func NewEmbeddingProvider(mode, endpoint, token string, timeout time.Duration) (domain.EmbeddingProvider, error) {
	p, e := newProvider(mode, endpoint, token, "", timeout)
	return &embeddings{p}, e
}

// Embed отправляет порцию текста и возвращает только числовые векторы; schema проверяется строго.
// @args ctx — deadline; request — версия модели и bounded inputs.
// @return векторы либо безопасная ошибка.
func (p *embeddings) Embed(ctx context.Context, request domain.EmbeddingRequest) ([][]float32, error) {
	if p.mode == "noop" {
		return nil, jobs.ErrSkip
	}
	if len(request.Inputs) < 1 || len(request.Inputs) > 64 || request.Dimensions < 8 || request.Dimensions > 2000 {
		return nil, jobs.Error{Code: "embedding_input_limit"}
	}
	if p.mode == "mock" {
		result := make([][]float32, len(request.Inputs))
		for i, text := range request.Inputs {
			result[i] = mockEmbedding(text, request.Dimensions)
		}
		return result, nil
	}
	raw, e := json.Marshal(request)
	if e != nil || len(raw) > 1<<20 {
		return nil, jobs.Error{Code: "embedding_input_limit"}
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if e != nil {
		return nil, jobs.Error{Code: "embedding_request"}
	}
	req.Header.Set("Content-Type", "application/json")
	data, e := p.call(req, request.IdempotencyKey, 2<<20)
	if e != nil {
		return nil, e
	}
	var result struct {
		Vectors [][]float32 `json:"vectors"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, jobs.Error{Code: "embedding_schema"}
	}
	return result.Vectors, nil
}

// mockEmbedding даёт детерминированный тестовый вектор с маленьким словарём синонимов, не настоящий semantic model.
// @args text — тестовый текст; dimensions — размерность фикстуры.
// @return нормированный feature-hash вектор для воспроизводимых интеграционных тестов.
func mockEmbedding(text string, dimensions int) []float32 {
	vector := make([]float32, dimensions)
	groups := map[string]string{"бюджет": "budget", "стоимость": "budget", "cost": "budget", "budget": "budget", "запуск": "launch", "релиз": "launch", "launch": "launch", "release": "launch", "встреча": "meeting", "совещание": "meeting", "meeting": "meeting"}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if normalized, ok := groups[word]; ok {
			word = normalized
		}
		hash := sha256.Sum256([]byte(word))
		index := int(binary.LittleEndian.Uint32(hash[:4]) % uint32(dimensions))
		vector[index]++
	}
	var norm float64
	for _, v := range vector {
		norm += float64(v * v)
	}
	if norm == 0 {
		vector[0] = 1
		return vector
	}
	norm = math.Sqrt(norm)
	for i := range vector {
		vector[i] /= float32(norm)
	}
	return vector
}
