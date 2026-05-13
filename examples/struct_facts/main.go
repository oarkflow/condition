package main

import (
	"fmt"
	"log"

	"github.com/oarkflow/condition"
)

type User struct {
	Age     int    `json:"age"`
	Tier    string `json:"tier"`
	Country string `json:"country"`
}

type RequestFacts struct {
	User User     `json:"user"`
	Tags []string `json:"tags"`
}

func main() {
	facts := condition.NewStructFacts(RequestFacts{
		User: User{Age: 41, Tier: "vip", Country: "NP"},
		Tags: []string{"beta", "trusted"},
	})

	result, err := condition.Eval(`user.age >= 40 and user.country == "NP" and "trusted" in tags`, facts)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("matched:", result.Matched)
}
