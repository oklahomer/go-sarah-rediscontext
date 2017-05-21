This is an [```sarah.UserContextStorage```](https://github.com/oklahomer/go-sarah) implementation that use Redis as its primary storage.

# Basic idea of user's conversational state: sarah.UserContext
One outstanding feature that ```Sarah``` offers is the ability to store user's conversational context, ```sarah.UserContext```, initiated by ```sarah.Command```.
With this feature, ```sarah.Command``` developer can let messaging user stay in the ```sarah.Command```'s conversation without adding any change to ```sarah.Bot``` or ```sarah.Runner``` logic.

When a ```sarah.Command``` returns ```sarah.CommandResponose``` with ```sarah.UserContext```,
```Sarah``` considers the user is in the middle of ```sarah.Command's``` conversational context and stores this context information in designated ```sarah.Storage```.
When the user sends next input within a pre-configured timeout window, the input is passed to the function defined by stored ```sarah.UserContext```.

This is how a ```sarah.Command``` turns typical one-response-per-input bot interaction to conversational one so users can input series of arguments in a more user-friendly conversational manner.

# sarah.UserContextStorage
Pre-defined default storage is provided and can be initialized via ```sarah.NewUserContextStorage```, but developers may replace it with preferred storage since ```sarah.UserContextStorage``` is merely an interface.
A use of alternative storage is indeed recommended for production environment for two reasons:

- Default storage internally uses a map to store ```sarah.UserContext``` in the process memory space, which means all stored contexts are vanished on process restart.
- While some chat services such as Slack and Gitter let bot initiate a connection against chat server, some such as LINE let chat services' server initiate HTTP request against bot server.
With this model, to handle larger amount of HTTP requests, bot may consist of multiple server instances.
Therefore multiple ```sarah.Bot``` processes over multiple server instances must be capable of sharing ```sarah.UserContextStorage``` to let user continue her conversation.

This repository provides one solution to store serialized ```sarah.UserContext``` in Redis.
One limitation to use external KVS is that arguments to the callback function must be serializable;
while default storage does not require so because it casually stores callback functions in Golang's map structure.

# Getting started
Below is a simple example that describes how to provide ```sarah.UserContext```-to-callback mapping, configure, and initialize storage.

```go
// Provide context-to-callback mapping
// When serialized argument and function ID of "callbackFuncID" are stored with the key of sarah.Input.SenderKey(),
// the function defined here is considered to correspond because this function is set with ID of "callbackFuncID" as second argument shows.
rediscontext.SetFunc(
        line.LINE,
        "callbackFuncID",
        reflect.TypeOf(&Hello{}).Elem(), // A type of argument to deserialize
        func(_ context.Context, input sarah.Input, arg interface{}) (*sarah.CommandResponse, error) {
                // Do something with given input and stored argument.
                return nil, nil
        },
)

// Configure
lineStorage := rediscontext.NewUserContextStorage(
        line.LINE,
        rediscontext.NewConfig(),
        &redis.Options{
                Addr:     "localhost:6379",
                Password: "",
                DB:       0,
        },
)
```

To store ```sarah.UserContext``` that kicks callbackFuncID on next user input, return ```sarah.UserContext``` as ```sarah.Command``` response.

```go
res :=  &sarah.CommandResponse{
        // Response content.
        Content: []linebot.Message{
                linebot.NewTextMessage("Hello! This is immediately returned to user."),
		},
		
		// Here defines the conversational context to be stored.
		UserContext: &sarah.UserContext{
                Serializable: &sarah.SerializableArgument{
                        FuncIdentifier: "callbackFuncID",
                        Argument: Hello{
                                Input: input.Message(),
                        },
                },
        }
```

