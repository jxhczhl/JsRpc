package utils

import "github.com/google/uuid"

func GetUUID() string {
	u := uuid.New()
	key := u.String()
	return key
}
