package server

import (
	"fmt"
	"time"

	"github.com/oklahomer/go-sarah-iot/auth"
	"github.com/patrickmn/go-cache"
)

// Response contains response content from device.
type Response struct {
	Content interface{}
	Device  *auth.Device
}

// TransactionStorageConfig contains configuration values for TransactionStorage.
type TransactionStorageConfig struct {
	ExpiresIn       time.Duration `json:"expires_in" yaml:"expires_in"`
	CleanupInterval time.Duration `json:"cleanup_interval" yaml:"cleanup_interval"`
}

// NewTransactionStorageConfig creates config object with default values.
// Use (json|yaml).Unmarshal to override default values.
func NewTransactionStorageConfig() *TransactionStorageConfig {
	return &TransactionStorageConfig{
		ExpiresIn:       3 * time.Minute,
		CleanupInterval: 10 * time.Minute,
	}
}

// TransactionStorage defines interface that all storage for transactional state must satisfy.
type TransactionStorage interface {
	Add(string, chan *Response)
	Get(string) (chan *Response, error)
}

type transactionStorage struct {
	cache *cache.Cache
}

var _ TransactionStorage = (*transactionStorage)(nil)

// NewTransactionStorage creates instance of default TransactionStorage with given configuration.
func NewTransactionStorage(config *TransactionStorageConfig) TransactionStorage {
	return &transactionStorage{
		cache: cache.New(config.ExpiresIn, config.CleanupInterval),
	}
}

func (s *transactionStorage) Add(key string, destination chan *Response) {
	s.cache.Set(key, destination, cache.DefaultExpiration)
}

func (s *transactionStorage) Get(key string) (chan *Response, error) {
	val, hasKey := s.cache.Get(key)
	if !hasKey || val == nil {
		return nil, fmt.Errorf("value not found with key: %s", key)
	}

	switch v := val.(type) {
	case chan *Response:
		return v, nil

	default:
		return nil, fmt.Errorf("cached value has illegal type of %T", v)

	}
}
