package model

type User struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Links    any    `json:"links,omitempty"`
}
