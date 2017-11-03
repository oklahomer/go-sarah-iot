package server

import (
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
)

func TestNewTransactionStorageConfig(t *testing.T) {
	config := NewTransactionStorageConfig()
	if config == nil {
		t.Errorf("TransactionalStorageConfig is not returned.")
	}
}

func TestNewTransactionStorage(t *testing.T) {
	config := &TransactionStorageConfig{}
	storage := NewTransactionStorage(config)

	if storage == nil {
		t.Fatalf("TransactionalStorage is not returned.")
	}

	typed, ok := storage.(*transactionStorage)

	if !(ok) {
		t.Fatal("transactionStorage is not returned.")
	}

	if typed.cache == nil {
		t.Error("cache is not set.")
	}
}

func TestTransactionStorage_Add(t *testing.T) {
	storage := &transactionStorage{
		cache: cache.New(3*time.Minute, 3*time.Minute),
	}

	key := "key1"
	responses := make(chan *Response)
	storage.Add(key, responses)

	ch, stored := storage.cache.Get(key)

	if !stored {
		t.Fatal("Value is not set.")
	}

	if ch == nil {
		t.Fatal("Stored value is not returned.")
	}

	typed, ok := ch.(chan *Response)
	if !ok {
		t.Errorf("Unexpected typed value is returned: %T", ch)
	}

	if typed != responses {
		t.Errorf("Expected channel is not returned: %#v", typed)
	}
}

func TestTransactionStorage_Get(t *testing.T) {
	storage := &transactionStorage{
		cache: cache.New(3*time.Minute, 3*time.Minute),
	}

	tests := []struct {
		key     string
		value   interface{}
		isValid bool
	}{
		{
			key:     "key1",
			value:   make(chan *Response),
			isValid: true,
		},
		{
			key:     "key2",
			value:   struct{}{},
			isValid: false,
		},
	}

	for i, test := range tests {
		i++

		storage.cache.Add(test.key, test.value, cache.DefaultExpiration)
		responses, err := storage.Get(test.key)
		if test.isValid && err != nil {
			t.Errorf("Unexpected error is returned on test #%d: %s", i, err.Error())
		}

		if test.isValid && responses == nil {
			t.Errorf("Expected channel is not returned on test #%d.", i)
		}

		if !test.isValid && err == nil {
			t.Errorf("Expected error is not returned on test #%d", i)
		}

		if !test.isValid && responses != nil {
			t.Errorf("Channel is returned unexpectedly on test #%d.", i)
		}
	}

}
