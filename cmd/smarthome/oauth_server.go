package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-oauth2/oauth2/v4/errors"
	"github.com/go-oauth2/oauth2/v4/generates"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-oauth2/oauth2/v4/store"
)

func ValidateJWT(r *http.Request) (bool, string) {
	reqToken := r.Header.Get("Authorization")
	if reqToken == "" {
		return false, "No Authorization header"
	}
	splitToken := strings.Split(reqToken, " ")
	access := splitToken[1]

	token, err := jwt.ParseWithClaims(access, &generates.JWTAccessClaims{},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Parse JWT error")
			}
			return []byte(os.Getenv("OAUTH_JWT_KEY")), nil
		})
	if err != nil {
		return false, "Unauthorized"
	}

	_, ok := token.Claims.(*generates.JWTAccessClaims)
	if !ok || !token.Valid {
		return false, "Invalid token"
	}

	return true, ""
}

func SetupOauth(mux *http.ServeMux) {
	manager := manage.NewDefaultManager()
	manager.MustTokenStorage(store.NewMemoryTokenStore())

	// We only have one OAuth client to populate, used by Google Smart Home
	// for https://developers.google.com/assistant/smarthome/overview
	clientStore := store.NewClientStore()
	clientId := os.Getenv("OAUTH_CLIENT")
	clientStore.Set(clientId, &models.Client{
		ID:     clientId,
		Secret: os.Getenv("OAUTH_SECRET"),
		Domain: "https://oauth-redirect.googleusercontent.com/",
	})
	manager.MapClientStorage(clientStore)

	// Running in Cloud Run, we'd like to allow Smart Home to authenticate once and
	// get a token from one of our instances, and be able to use that token with any
	// running instance. We'd have to run a central DB somewhere to store sessions,
	// Firebase maybe, but instead we use JWT to make out tokens using a secret
	// which Cloud Run passes in from the environment. https://jwt.io/
	// Any of our instances can validate the token created by any other instance.
	jwt_key := []byte(os.Getenv("OAUTH_JWT_KEY"))
	manager.MapAccessGenerate(generates.NewJWTAccessGenerate("", jwt_key,
		jwt.SigningMethodHS512))

	// Our interactions with users are in the form of smart home commands like
	// turning lights on. We'd like to minimize the latency from the time when the
	// user says "turn the light on" until the light turns on, so let the Access
	// Token live a long time to not spend round trips to refresh it.
	// Note that Google's server-side code discards this token well before the long
	// expiration time we give here. We just don't want to be the limit.
	manager.SetAuthorizeCodeExp(time.Minute * 10)
	cfg := &manage.Config{
		AccessTokenExp:    time.Hour * 24 * 7,
		RefreshTokenExp:   time.Hour * 24 * 7,
		IsGenerateRefresh: true,
	}
	manager.SetAuthorizeCodeTokenCfg(cfg)

	srv := server.NewDefaultServer(manager)
	srv.SetAllowGetAccessRequest(true)
	srv.SetClientInfoHandler(server.ClientFormHandler)

	srv.SetInternalErrorHandler(func(err error) (re *errors.Response) {
		log.Println("Internal Error:", err.Error())
		return
	})

	srv.SetResponseErrorHandler(func(re *errors.Response) {
		log.Printf("Response Error: Error=%v, Description=%v, URI=%v\n",
			re.Error.Error(), re.Description, re.URI)
	})

	// We don't want to spin up a shared session database like Firebase, and we only
	// have one OAuth client which only has one user. Return that user, always.
	srv.SetUserAuthorizationHandler(
		func(w http.ResponseWriter, r *http.Request) (userID string, err error) {
			return "google_smart_home", nil
		})

	// instantiate handlers on our HTTP server
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		err := srv.HandleAuthorizeRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		srv.HandleTokenRequest(w, r)
	})
}
