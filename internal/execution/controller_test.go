package execution

import (
	"context"
	"testing"
	"time"
)

func TestControllerRejectsWhenConcurrencyIsExhausted(t *testing.T) {
	controller := New(map[Operation]Budget{Query: {Timeout: time.Second, Concurrency: 1}})
	_, release, ok := controller.Start(context.Background(), Query)
	if !ok || release == nil {
		t.Fatal("first operation was not admitted")
	}
	defer release()
	if _, blockedRelease, ok := controller.Start(context.Background(), Query); ok || blockedRelease != nil {
		t.Fatal("second operation was admitted despite an exhausted budget")
	}
}

func TestControllerDeadlineCancelsOperationContext(t *testing.T) {
	controller := New(map[Operation]Budget{Evaluation: {Timeout: 10 * time.Millisecond, Concurrency: 1}})
	ctx, release, ok := controller.Start(context.Background(), Evaluation)
	if !ok {
		t.Fatal("operation was not admitted")
	}
	defer release()
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("context error = %v", ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("operation context did not time out")
	}
}

func TestControllerUsesIndependentOperationSlots(t *testing.T) {
	controller := New(map[Operation]Budget{
		Ingestion: {Timeout: time.Second, Concurrency: 1},
		Query:     {Timeout: time.Second, Concurrency: 1},
	})
	_, releaseIngestion, ok := controller.Start(context.Background(), Ingestion)
	if !ok {
		t.Fatal("ingestion was not admitted")
	}
	defer releaseIngestion()
	_, releaseQuery, ok := controller.Start(context.Background(), Query)
	if !ok {
		t.Fatal("query incorrectly shared the ingestion slot")
	}
	defer releaseQuery()
}
