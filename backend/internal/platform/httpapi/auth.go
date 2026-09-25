package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	maxbot "github.com/max-messenger/max-bot-api-client-go/v2"

	"maxhackathon/backend/internal/access"
)

// Header names. The mini-app sends window.WebApp.initData as is in HeaderInitData.
const (
	HeaderInitData      = "X-Max-Init-Data"
	HeaderDevUserID     = "X-Dev-User-Id"
	HeaderDevStartParam = "X-Dev-Start-Param"
)

// maxClockSkew tolerates a launch time slightly in the future (client clock drift).
const maxClockSkew = 5 * time.Minute

// Identity is the authenticated caller of an API request.
type Identity struct {
	UserID     string
	MaxUserID  int64
	FirstName  string
	StartParam string
	Dev        bool
}

type identityKey struct{}

// IdentityFrom returns the identity put into the context by the auth middleware.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)

	return id, ok
}

// Users creates or finds users by MAX id (the access module).
type Users interface {
	UpsertUser(ctx context.Context, maxUserID int64) (access.User, error)
}

// Authenticator checks every API request.
//
// A request is accepted only with a valid initData signed by MAX (HMAC with the bot
// token) that is not older than maxAge. In dev mode a request without initData may
// instead name a user in X-Dev-User-Id; dev mode must never run where real users are.
type Authenticator struct {
	BotToken string
	MaxAge   time.Duration
	DevMode  bool
	Users    Users
	Log      *slog.Logger
	Now      func() time.Time
}

// Middleware authenticates the request and puts Identity into its context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.identify(w, r)
		if !ok {
			return
		}

		u, err := a.Users.UpsertUser(r.Context(), id.MaxUserID)
		if err != nil {
			a.Log.Error("upsert user", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "Не удалось обработать запрос, попробуйте ещё раз")

			return
		}
		id.UserID = u.ID

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, id)))
	})
}

func (a *Authenticator) identify(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	if initData := r.Header.Get(HeaderInitData); initData != "" {
		return a.fromInitData(w, initData)
	}

	if a.DevMode && r.Header.Get(HeaderDevUserID) != "" {
		maxID, err := strconv.ParseInt(r.Header.Get(HeaderDevUserID), 10, 64)
		if err != nil || maxID <= 0 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Некорректный X-Dev-User-Id")

			return Identity{}, false
		}

		return Identity{
			MaxUserID:  maxID,
			FirstName:  "Разработчик",
			StartParam: r.Header.Get(HeaderDevStartParam),
			Dev:        true,
		}, true
	}

	writeError(w, http.StatusUnauthorized, "unauthorized", "Откройте приложение из MAX")

	return Identity{}, false
}

func (a *Authenticator) fromInitData(w http.ResponseWriter, initData string) (Identity, bool) {
	if a.BotToken == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Сервер запущен без токена бота")

		return Identity{}, false
	}

	data, err := maxbot.ValidateInitData(initData, a.BotToken)
	if err != nil || data.User.ID <= 0 {
		// The reason is logged, not returned: it helps an attacker more than a user.
		a.Log.Warn("invalid initData", "err", err)
		writeError(w, http.StatusUnauthorized, "invalid_init_data", "Не удалось проверить запуск из MAX, откройте приложение заново")

		return Identity{}, false
	}

	now := a.Now()
	authAt := time.Unix(data.AuthDate, 0)
	if now.Sub(authAt) > a.MaxAge || authAt.Sub(now) > maxClockSkew {
		writeError(w, http.StatusUnauthorized, "init_data_expired", "Сессия устарела, откройте приложение заново")

		return Identity{}, false
	}

	return Identity{
		MaxUserID:  data.User.ID,
		FirstName:  data.User.FirstName,
		StartParam: data.StartParam,
	}, true
}
