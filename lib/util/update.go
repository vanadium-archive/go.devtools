package util

type Update map[string][]Commit

type Commit struct {
	Author      string
	Email       string
	Description string
}
