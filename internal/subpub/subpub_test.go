package subpub_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"publisher_subscriber/internal/subpub"
)

func TestSubscribe(t *testing.T) {
	sp := subpub.NewSubPub()

	subject := "test"

	sub, err := sp.Subscribe(subject, func(msg interface{}) {})
	if err != nil {
		t.Error(err)
	}
	if sub == nil {
		t.Error("Subscribe nil")
	}

	ctx := context.Background()
	err = sp.Close(ctx)
	if err != nil {
		t.Error(err)
	}

	_, err = sp.Subscribe(subject, func(interface{}) {})
	if !errors.Is(err, subpub.ErrClosed) {
		t.Error(err)
	}
}

func TestUnsubscribe(t *testing.T) {
	sp := subpub.NewSubPub()

	subject := "test"

	sub, err := sp.Subscribe(subject, func(msg interface{}) {})
	if err != nil {
		t.Error(err)
	}
	sub.Unsubscribe()

	sub.Unsubscribe()

	sub, err = sp.Subscribe(subject, func(msg interface{}) {})
	if err != nil {
		t.Error(err)
	}

	ctx := context.Background()
	err = sp.Close(ctx)
	if err != nil {
		t.Error(err)
	}

	sub.Unsubscribe()
}

func TestPublish(t *testing.T) {
	sp := subpub.NewSubPub()

	var msgReceives bool
	subject := "test"
	thisMsg := "hello VK"

	var wg sync.WaitGroup
	wg.Add(1)

	_, err := sp.Subscribe(subject, func(msg interface{}) {
		if msg == thisMsg {
			msgReceives = true
		}
		wg.Done()
	})
	if err != nil {
		t.Error(err)
	}

	err = sp.Publish(subject, thisMsg)
	if err != nil {
		t.Error(err)
	}

	wg.Wait()
	if !msgReceives {
		t.Error("Message not received")
	}

	err = sp.Publish("errorSubject", thisMsg)
	if !errors.Is(err, subpub.ErrNoSubscribers) {
		t.Error(err)
	}

	ctx := context.Background()
	err = sp.Close(ctx)
	if err != nil {
		t.Error(err)
	}

	err = sp.Publish(subject, thisMsg)
	if !errors.Is(err, subpub.ErrClosed) {
		t.Error(err)
	}
}

func TestClose(t *testing.T) {
	sp := subpub.NewSubPub()

	ctx := context.Background()
	err := sp.Close(ctx)
	if err != nil {
		t.Error(err)
	}

	err = sp.Close(ctx)
	if err != nil {
		t.Error(err)
	}

	sp = subpub.NewSubPub()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = sp.Close(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

func TestConcurrentOperations(t *testing.T) {
	sp := subpub.NewSubPub()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			subject := "test"
			thisMsg := "hello VK"

			sub, err := sp.Subscribe(subject, func(msg interface{}) {})
			if err != nil {
				t.Error(err)
			}

			err = sp.Publish(subject, thisMsg)
			if err != nil {
				t.Error(err)
			}

			sub.Unsubscribe()
		}()
	}

	wg.Wait()
	ctx := context.Background()
	err := sp.Close(ctx)
	if err != nil {
		t.Error(err)
	}
}

func TestSlowSubscriber(t *testing.T) {
	sp := subpub.NewSubPub()
	var wg sync.WaitGroup

	wg.Add(1)

	subject := "test"
	thisMsg := "hello VK"

	_, err := sp.Subscribe(subject, func(msg interface{}) {
		time.Sleep(time.Second)
		wg.Done()
	})
	if err != nil {
		t.Error(err)
	}

	var fastReceived bool
	_, err = sp.Subscribe(subject, func(msg interface{}) {
		fastReceived = true
	})
	if err != nil {
		t.Error(err)
	}

	err = sp.Publish(subject, thisMsg)
	if err != nil {
		t.Error(err)
	}

	wg.Wait()
	if !fastReceived {
		t.Error("Slow subscriber did not receive message")
	}

	ctx := context.Background()
	err = sp.Close(ctx)
	if err != nil {
		t.Error(err)
	}
}

func TestBufferOverflow(t *testing.T) {
	sp := subpub.NewSubPub()
	subject := "test"
	const numMessages = subpub.BufferSize

	processed := make(chan struct{}, numMessages*2)
	defer close(processed)

	var processedCount int32

	_, err := sp.Subscribe(subject, func(msg interface{}) {
		atomic.AddInt32(&processedCount, 1)
		processed <- struct{}{}
		time.Sleep(50 * time.Millisecond)
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < numMessages; i++ {
		err := sp.Publish(subject, i)
		if err != nil {
			t.Fatal(err)
		}
	}

	for i := 0; i < numMessages; i++ {
		select {
		case <-processed:
		case <-time.After(1 * time.Second):
			t.Errorf("timeout waiting for message %d", i)
		}
	}

	overflowMsg := "overflow"
	err = sp.Publish(subject, overflowMsg)
	if err != nil {
		t.Error("Publish should not return error on full buffer")
	}

	time.Sleep(200 * time.Millisecond)

	if count := atomic.LoadInt32(&processedCount); count != numMessages+1 {
		t.Errorf("Expected exactly %d processed messages, got %d", numMessages, count)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := sp.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
