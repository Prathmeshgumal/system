package httpx

import "context"

type ctxKey int

const userIDKey ctxKey = iota

// WithUserID records who is calling. Set by the auth middleware.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// UserID returns the caller's user id. Handlers behind the auth middleware
// can rely on it being set; anywhere else it returns 0.
func UserID(ctx context.Context) int64 {
	id, _ := ctx.Value(userIDKey).(int64)
	return id
}
