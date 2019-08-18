package rediscontext

import (
	"context"
	"encoding/json"
	"github.com/go-redis/redis"
	"github.com/oklahomer/go-sarah"
	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func SetupAndRun(fnc func()) {
	// Initialize package variables
	stashedFunc = &funcStash{}

	fnc()
}

type DummyArg struct {
	Bar string `json:"bar"`
}

type DummyClient struct {
	getFunc      func(string) ([]byte, error)
	setFunc      func(string, interface{}, time.Duration) error
	delFunc      func(...string) error
	flushAllFunc func() error
}

var _ client = (*DummyClient)(nil)

func (c *DummyClient) Get(key string) ([]byte, error) {
	return c.getFunc(key)
}

func (c *DummyClient) Set(key string, value interface{}, expiration time.Duration) error {
	return c.setFunc(key, value, expiration)
}

func (c *DummyClient) Del(keys ...string) error {
	return c.delFunc(keys...)
}

func (c *DummyClient) FlushAll() error {
	return c.flushAllFunc()
}

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	if config == nil {
		t.Fatal("Returned instance is nil.")
	}
}

func TestNewUserContextStorage(t *testing.T) {
	SetupAndRun(func() {
		var botType sarah.BotType = "dummyBot"
		config := NewConfig()
		redisOptions := &redis.Options{}

		storage := NewUserContextStorage(botType, config, redisOptions)

		if storage == nil {
			t.Fatal("Returned instance is nil.")
		}

		redisStorage, ok := storage.(*userContextStorage)
		if !ok {
			t.Fatal("Returned instance is not type of userContextStorage.")
		}

		if redisStorage.botType != botType {
			t.Errorf("Unexpected BotType is returned: %s.", redisStorage.botType)
		}

		if redisStorage.expiresIn != config.ExpiresIn {
			t.Errorf("Unexpected expiration time is returned: %d.", redisStorage.expiresIn)
		}

		if _, ok := redisStorage.client.(*redisClient); !ok {
			t.Errorf("Returned client is not type of redis.Clilent: %T.", redisStorage.client)
		}
	})
}

func TestSetFunc(t *testing.T) {
	SetupAndRun(func() {
		var botType sarah.BotType = "dummyBot"
		funcId := "myFunc"
		argType := reflect.TypeOf(&DummyArg{}).Elem()
		fnc := func(_ context.Context, _ sarah.Input, _ interface{}) (*sarah.CommandResponse, error) {
			return nil, nil
		}

		SetFunc(botType, funcId, argType, fnc)

		stash, ok := (*stashedFunc)[botType]
		if !ok {
			t.Fatal("No function is stashed with given BotType.")
		}

		if len(stash) != 1 {
			t.Fatalf("Size of stashed function is not 1: %d", len(stash))
		}

		if stash[0].argType != argType {
			t.Errorf("Unexpected argType is set: %#v.", stash[0].argType)
		}

		if reflect.ValueOf(stash[0].fnc).Pointer() != reflect.ValueOf(fnc).Pointer() {
			t.Error("Unexpected function is set.")
		}

		if stash[0].identifier != funcId {
			t.Errorf("Unexpected identifier is set: %s.", stash[0].identifier)
		}
	})
}

func TestUserContextStorage_Set(t *testing.T) {
	SetupAndRun(func() {
		funcID := "foo"
		tests := []struct {
			key string
			ctx *sarah.UserContext
			err bool
		}{
			{
				key: "user123",
				ctx: &sarah.UserContext{},
				err: true,
			},
			{
				key: "user123",
				ctx: nil,
				err: true,
			},
			{
				key: "user123",
				ctx: &sarah.UserContext{
					Next: func(_ context.Context, _ sarah.Input) (*sarah.CommandResponse, error) {
						return nil, nil
					},
				},
				err: true,
			},
			{
				key: "user123",
				ctx: &sarah.UserContext{
					Serializable: &sarah.SerializableArgument{
						FuncIdentifier: funcID,
						Argument: &DummyArg{
							Bar: "bar",
						},
					},
				},
				err: false,
			},
		}

		for i, tt := range tests {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				var givenKey string
				var givenValue interface{}
				var givenExpiration time.Duration
				expiration := time.Minute
				client := &DummyClient{
					setFunc: func(key string, value interface{}, expiresIn time.Duration) error {
						givenKey = key
						givenExpiration = expiration
						givenValue = value
						return nil
					},
				}
				storage := &userContextStorage{
					botType:   "dummyBot",
					expiresIn: expiration,
					client:    client,
				}

				err := storage.Set(tt.key, tt.ctx)

				if tt.err {
					if err == nil {
						t.Fatalf("Expected error is not returned.")
					}
					return
				}

				if givenKey != tt.key {
					t.Errorf("Expected key is not passed: %s", givenKey)
				}

				arg := &JsonArgument{}
				_ = json.Unmarshal(givenValue.([]byte), arg)
				if arg.FuncIdentifier != funcID {
					t.Errorf("Expected function identifier is not passed: %#v", arg.FuncIdentifier)
				}
				res := gjson.ParseBytes(givenValue.([]byte))
				mapped := &DummyArg{}
				err = json.Unmarshal([]byte(res.Get("argument").Raw), mapped)
				if mapped.Bar != tt.ctx.Serializable.Argument.(*DummyArg).Bar {
					t.Errorf("Expected argument is not passed: %#v", arg.Argument)
				}

				if givenExpiration != expiration {
					t.Errorf("Expected expiration is not passed: %#v", givenExpiration)
				}
			})
		}
	})
}

func TestUserContextStorage_Get(t *testing.T) {
	tests := []struct {
		stored      string
		stashedFunc *funcStash
		error       bool
		botType     sarah.BotType
	}{
		{
			stored:  ``,
			botType: "botType",
			error:   false,
		},
		{
			stored:  `{}`,
			botType: "botType",
			error:   true,
		},
		{
			stored:      `{"func_identifier": "dummyID", "argument": null}`,
			stashedFunc: &funcStash{"invalidBotType": []*fncContainer{}},
			botType:     "dummyID",
			error:       true,
		},
		{
			stored: `{"func_identifier": "dummyID", "argument": null}`,
			stashedFunc: &funcStash{"botType": []*fncContainer{
				{
					identifier: "dummyID",
					argType:    reflect.TypeOf(&DummyArg{}),
					fnc: func(_ context.Context, _ sarah.Input, _ interface{}) (*sarah.CommandResponse, error) {
						return nil, nil
					},
				},
			}},
			botType: "botType",
			error:   false,
		},
	}

	for i, tt := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			stashedFunc = tt.stashedFunc
			client := &DummyClient{
				getFunc: func(_ string) ([]byte, error) {
					stored := tt.stored
					if stored == "" {
						return nil, redis.Nil
					}
					return []byte(stored), nil
				},
			}
			storage := &userContextStorage{
				botType: "botType",
				client:  client,
			}

			contextualFunc, err := storage.Get("key")
			if tt.error {
				if err == nil {
					t.Fatal("Expected error is not returned.")
				}
				return
			}

			if tt.stashedFunc == nil {
				return
			}

			if contextualFunc == nil {
				t.Error("Expected function is not returned.")
			}
		})
	}

	t.Run("Redis error", func(t *testing.T) {
		returningErr := xerrors.New("redis error")
		client := &DummyClient{
			getFunc: func(_ string) ([]byte, error) {
				return []byte{}, returningErr
			},
		}
		storage := &userContextStorage{
			client: client,
		}

		contextualFunc, err := storage.Get("key")
		if !xerrors.Is(err, returningErr) {
			t.Fatalf("Expected error is not returned: %#v", err)
		}

		if contextualFunc != nil {
			t.Error("Contextual function should not return.")
		}
	})
}

func TestUserContextStorage_Delete(t *testing.T) {
	SetupAndRun(func() {
		var givenKeys []string
		client := &DummyClient{
			delFunc: func(keys ...string) error {
				givenKeys = keys
				return nil
			},
		}
		storage := &userContextStorage{
			client: client,
		}

		target := "targetKey"
		err := storage.Delete(target)
		if err != nil {
			t.Fatalf("Unexpected error is returned: %s", err.Error())
		}

		if len(givenKeys) != 1 {
			t.Errorf("Unexpected number of keys were given: %d", len(givenKeys))
		}

		if givenKeys[0] != target {
			t.Errorf("Unexpected key is given: %s", givenKeys[0])
		}
	})
}

func TestUserContextStorage_Flush(t *testing.T) {
	SetupAndRun(func() {
		called := false
		client := &DummyClient{
			flushAllFunc: func() error {
				called = true
				return nil
			},
		}
		storage := &userContextStorage{
			client: client,
		}

		err := storage.Flush()
		if err != nil {
			t.Fatalf("Unexpected error is returned: %s", err.Error())
		}

		if !called {
			t.Error("Flush method is not called.")
		}
	})
}
