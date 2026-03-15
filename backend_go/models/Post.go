package models

type Post struct {
	Id      int    `json:"id"`
	UserId  int    `json:"userId"`
	Content string `json:"content"`
}
