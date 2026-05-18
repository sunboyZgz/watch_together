package transport

import "watch_together/server/internal/auth"

func testAuthorizationHeader(userID string) string {
	token, err := auth.NewTokenManager(auth.DefaultTokenConfig()).IssueAccessToken(auth.User{ID: userID})
	if err != nil {
		panic(err)
	}
	return "Bearer " + token
}
