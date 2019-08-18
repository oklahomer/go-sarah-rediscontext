package rediscontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/oklahomer/go-sarah"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"
	"reflect"
	"time"
)

var stashedFunc = &funcStash{}

var (
	// ErrInvalidUserContext indicates that malformed sarah.UserContext is passed and operation can not be performed.
	ErrInvalidUserContext = errors.New("user context or its holding argument is nil")
)

type funcStash map[sarah.BotType][]*fncContainer

func (stash *funcStash) get(botType sarah.BotType, identifier string) (*fncContainer, error) {
	fncContainers, ok := (*stash)[botType]
	if !ok {
		return nil, fmt.Errorf("no function is stashed for BotType: %s", botType)
	}

	for _, container := range fncContainers {
		if container.identifier == identifier {
			return container, nil
		}
	}

	return nil, nil
}

type fncContainer struct {
	identifier string
	argType    reflect.Type
	fnc        func(context.Context, sarah.Input, interface{}) (*sarah.CommandResponse, error)
}

// SetFunc stores given fnc with corresponding id.
func SetFunc(botType sarah.BotType, id string, argType reflect.Type, fnc func(context.Context, sarah.Input, interface{}) (*sarah.CommandResponse, error)) {
	stash := *stashedFunc
	if _, ok := stash[botType]; !ok {
		stash[botType] = make([]*fncContainer, 0)
	}

	stash[botType] = append(stash[botType], &fncContainer{
		identifier: id,
		argType:    argType,
		fnc:        fnc,
	})
}

type client interface {
	Get(string) ([]byte, error)
	Set(string, interface{}, time.Duration) error
	Del(...string) error
	FlushAll() error
}

type redisClient struct {
	c *redis.Client
}

var _ client = (*redisClient)(nil)

func (r *redisClient) Get(key string) ([]byte, error) {
	return r.c.Get(key).Bytes()
}

func (r *redisClient) Set(key string, data interface{}, ex time.Duration) error {
	return r.c.Set(key, data, ex).Err()
}

func (r *redisClient) Del(keys ...string) error {
	return r.c.Del(keys...).Err()
}

func (r *redisClient) FlushAll() error {
	return r.c.FlushAll().Err()
}

// Config contains some configuration variables.
type Config struct {
	ExpiresIn time.Duration `json:"expires_in" yaml:"expires_in"`
}

// NewConfig creates and returns new Config instance with default settings.
// Use json.Unmarshal, yaml.Unmarshal, or manual manipulation to override default values.
func NewConfig() *Config {
	return &Config{
		ExpiresIn: 5 * time.Minute,
	}
}

type userContextStorage struct {
	botType   sarah.BotType
	client    client
	expiresIn time.Duration
}

var _ sarah.UserContextStorage = (*userContextStorage)(nil)

// NewUserContextStorage initializes UserContextStorage implementation.
func NewUserContextStorage(botType sarah.BotType, config *Config, redisOptions *redis.Options) sarah.UserContextStorage {
	return &userContextStorage{
		botType:   botType,
		expiresIn: config.ExpiresIn,
		client:    &redisClient{c: redis.NewClient(redisOptions)},
	}
}

func (storage *userContextStorage) Get(key string) (sarah.ContextualFunc, error) {
	b, err := storage.client.Get(key)
	if err == redis.Nil {
		// Key does not exist.
		// User context is not stored.
		return nil, nil
	} else if err != nil {
		return nil, xerrors.Errorf("failed to fetch state from redis: %w", err)
	}

	res := gjson.ParseBytes(b)
	identifier := res.Get("func_identifier")
	if !identifier.Exists() {
		return nil, xerrors.Errorf("mandatory field, func_identifier, is not set in %s", b)
	}

	container, err := stashedFunc.get(storage.botType, identifier.String())
	if err != nil {
		return nil, err
	}

	// http://stackoverflow.com/a/18297937/694061
	arg := reflect.New(container.argType)
	err = json.Unmarshal([]byte(res.Get("argument").Raw), arg.Interface())
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
		return container.fnc(ctx, input, arg)
	}, nil
}

func (storage *userContextStorage) Set(key string, userContext *sarah.UserContext) error {
	if userContext == nil ||
		userContext.Serializable == nil ||
		userContext.Serializable.FuncIdentifier == "" ||
		userContext.Serializable.Argument == nil {

		return ErrInvalidUserContext
	}

	arg := &JsonArgument{
		FuncIdentifier: userContext.Serializable.FuncIdentifier,
		Argument:       userContext.Serializable.Argument,
	}

	b, err := json.Marshal(arg)
	if err != nil {
		return err
	}

	return storage.client.Set(key, b, storage.expiresIn)
}

func (storage *userContextStorage) Delete(key string) error {
	return storage.client.Del(key)
}

func (storage *userContextStorage) Flush() error {
	return storage.client.FlushAll()
}

// JsonArgument is a serializable argument to be stored in Redis.
// When the next input is sent from the same user, this argument is retrieved from Redis to continue the conversation.
type JsonArgument struct {
	FuncIdentifier string      `json:"func_identifier"`
	Argument       interface{} `json:"argument"`
}
