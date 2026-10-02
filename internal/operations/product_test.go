package operations

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
)

// TestProductMetricsBoundedLabels проверяет, что пользовательские строки не создают серии.
// @args t — контекст проверки ограниченной кардинальности и классификации ошибок.
func TestProductMetricsBoundedLabels(t *testing.T) {
	r := New("test", "test", config.OperationsConfig{}, nil)
	for i := 0; i < 100; i++ {
		private := uuid.NewString()
		Product(private, "done", time.Second)
		Product("content.transcribe", private, time.Second)
		ProductQueue(private, "queued", 1)
		ProductQueue("content.transcribe", private, 1)
		ProviderCall(private, time.Second, nil)
	}
	Product("content.transcribe", "done", time.Second)
	ProductQueue("content.transcribe", "queued", 2)
	for _, err := range []error{nil, jobs.ErrSkip, context.DeadlineExceeded,
		jobs.Error{Code: "provider_rate_limited", Retryable: true},
		fmt.Errorf("wrapped: %w", &jobs.Error{Code: "provider_rejected"}), errors.New("private arbitrary error")} {
		ProviderCall("stt", time.Second, err)
	}
	Search(time.Second, false)
	Search(time.Second, true)
	families, err := r.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]int{"recorder_product_jobs_total": 1, "recorder_product_queue": 1,
		"recorder_product_provider_calls_total": 6, "recorder_product_provider_duration_seconds": 1,
		"recorder_search_requests_total": 2, "recorder_search_duration_seconds": 1}
	for _, family := range families {
		if size, ok := expected[family.GetName()]; ok {
			if len(family.Metric) != size {
				t.Fatalf("%s: got %d series, want %d", family.GetName(), len(family.Metric), size)
			}
			delete(expected, family.GetName())
		}
	}
	if len(expected) != 0 {
		t.Fatal("missing product metrics", expected)
	}
}
