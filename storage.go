package rediscontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oklahomer/go-sarah"
	"github.com/tidwall/gjson"
	"golang.org/x/net/context"
	"gopkg.in/redis.v6"
	"reflect"
	"time"
)

var stashedFunc = &funcStash{}

type funcStash map[sarah.BotType][]*fncContainer

func (stash *funcStash) get(botType sarah.BotType, identifier string) (*fncContainer, error) {
	fncContainers, ok := (*stash)[botType]
	if !ok {
		return nil, fmt.Errorf("No function is stashed for BotType: %s.", botType)
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

type userContextStorage struct {
	botType   sarah.BotType
	client    *redis.Client
	expiresIn time.Duration
}

func NewUserContextStorage(botType sarah.BotType, redisOptions *redis.Options) sarah.UserContextStorage {
	return &userContextStorage{
		botType: botType,
		client:  redis.NewClient(redisOptions),
	}
}

type JsonArgument struct {
	FuncIdentifier string      `json:"func_identifier"`
	Argument       interface{} `json:"argument"`
}

func (storage *userContextStorage) Get(key string) (sarah.ContextualFunc, error) {
	b, err := storage.client.Get(key).Bytes()
	if err != nil {
		//return nil, err
		return nil, nil
	}

	if len(b) == 0 {
		return nil, nil
	}

	res := gjson.ParseBytes(b)
	identifier := res.Get("func_identifier")
	if !identifier.Exists() {
		return nil, fmt.Errorf("Mandatory field, func_identifier, is not set: %s", b)
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
	if userContext == nil {
		return errors.New("Storing UserContext is nil.")
	}

	if userContext.Serializable == nil {
		return errors.New("Serializable argument is not set.")
	}

	arg := &JsonArgument{
		FuncIdentifier: userContext.Serializable.FuncIdentifier,
		Argument:       userContext.Serializable.Argument,
	}

	b, err := json.Marshal(arg)
	if err != nil {
		return err
	}

	cmd := storage.client.Set(key, b, storage.expiresIn)
	return cmd.Err()
}

func (storage *userContextStorage) Delete(key string) error {
	cmd := storage.client.Del(key)
	return cmd.Err()
}

func (storage *userContextStorage) Flush() error {
	cmd := storage.client.FlushAll()
	return cmd.Err()
}
