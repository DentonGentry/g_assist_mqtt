package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

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

	clientStore := store.NewClientStore()
	clientId := os.Getenv("OAUTH_CLIENT")
	clientStore.Set(clientId, &models.Client{
		ID:     clientId,
		Secret: os.Getenv("OAUTH_SECRET"),
		Domain: "https://oauth-redirect.googleusercontent.com/",
	})
	manager.MapClientStorage(clientStore)

	jwt_key := []byte(os.Getenv("OAUTH_JWT_KEY"))
	manager.MapAccessGenerate(generates.NewJWTAccessGenerate("", jwt_key,
		jwt.SigningMethodHS512))

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

	srv.SetUserAuthorizationHandler(func(w http.ResponseWriter, r *http.Request) (userID string, err error) {
		// TODO: need to figure out the right thing to do here.
		return "123456", nil
	})

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
