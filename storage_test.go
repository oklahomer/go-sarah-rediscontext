package rediscontext

import (
	"encoding/json"
	"github.com/oklahomer/go-sarah"
	"golang.org/x/net/context"
	"gopkg.in/redis.v6"
	"reflect"
	"testing"
	"time"
)

type DummyArg struct {
	Bar string `json:"bar"`
}

type DummyClient struct {
	getFunc      func(string) *redis.StringCmd
	setFunc      func(string, interface{}, time.Duration) *redis.StatusCmd
	delFunc      func(...string) *redis.IntCmd
	flushAllFunc func() *redis.StatusCmd
}

func (c *DummyClient) Get(key string) *redis.StringCmd {
	return c.getFunc(key)
}

func (c *DummyClient) Set(key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	return c.setFunc(key, value, expiration)
}

func (c *DummyClient) Del(keys ...string) *redis.IntCmd {
	return c.delFunc(keys...)
}

func (c *DummyClient) FlushAll() *redis.StatusCmd {
	return c.flushAllFunc()
}

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	if config == nil {
		t.Fatal("Retrned instance is nil.")
	}
}

func TestNewUserContextStorage(t *testing.T) {
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

	if _, ok := redisStorage.client.(*redis.Client); !ok {
		t.Errorf("Returned client is not type of redis.Clilent: %T.", redisStorage.client)
	}
}

func TestSetFunc(t *testing.T) {
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
		t.Errorf("Unexpected function is set: %#v.", stash[0].fnc)
	}

	if stash[0].identifier != funcId {
		t.Errorf("Unexpected identifier is set: %s.", stash[0].identifier)
	}
}

func TestUserContextStorage_Set(t *testing.T) {
	var botType sarah.BotType = "dummyBot"
	senderKey := "user123"
	expiration := time.Minute

	var givenKey string
	var givenValue interface{}
	var givenExpiration time.Duration
	client := &DummyClient{
		setFunc: func(key string, value interface{}, expiresIn time.Duration) *redis.StatusCmd {
			givenKey = key
			givenExpiration = expiration
			givenValue = value
			return &redis.StatusCmd{}
		},
	}
	storage := &userContextStorage{
		botType:   botType,
		expiresIn: expiration,
		client:    client,
	}

	funcId := "foo"
	arg := &DummyArg{
		Bar: "bar",
	}
	err := storage.Set(
		senderKey,
		&sarah.UserContext{
			Serializable: &sarah.SerializableArgument{
				FuncIdentifier: funcId,
				Argument:       arg,
			},
		},
	)

	if err != nil {
		t.Fatalf("Unexpected error is returned: %s.", err.Error())
	}

	if givenKey != senderKey {
		t.Errorf("Unexpected sender key is given: %s.", givenKey)
	}

	if b, ok := givenValue.([]byte); ok {
		decodedValue := &JsonArgument{}
		if err := json.Unmarshal(b, decodedValue); err == nil {
			if decodedValue.FuncIdentifier != funcId {
				t.Errorf("Unexpected function identifier is given: %s.", b)
			}

		} else {
			t.Errorf("Unexpected json.Unmarshal error: %#v.", b)

		}

	} else {
		t.Errorf("Unexpected value type is given: %#v", givenValue)

	}

	if givenExpiration != expiration {
		t.Errorf("Unexpected expiration time is given: %d.", givenExpiration)
	}
}

func TestUserContextStorage_Set_ErrorWithNilContext(t *testing.T) {
	storage := &userContextStorage{}

	err := storage.Set("foo", nil)

	if err == nil {
		t.Fatal("Expected error is not returned.")
	}

	if err != ErrInvalidUserContext {
		t.Errorf("Expected error is not returned: %#v.", err)
	}
}
