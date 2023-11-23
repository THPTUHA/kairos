package deliverer

import "context"

type Credentials struct {
	UserID   string
	ExpireAt int64
	Info     []byte
}

type credentialsContextKeyType int

var credentialsContextKey credentialsContextKeyType

func SetCredentials(ctx context.Context, cred *Credentials) context.Context {
	ctx = context.WithValue(ctx, credentialsContextKey, cred)
	return ctx
}

func GetCredentials(ctx context.Context) (*Credentials, bool) {
	if val := ctx.Value(credentialsContextKey); val != nil {
		cred, ok := val.(*Credentials)
		return cred, ok
	}
	return nil, false
}
