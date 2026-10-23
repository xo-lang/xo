// Go twin of bench/perf/jsontwin: encoding/json decode and encode of 1000
// users, 100 rounds each. Usage: jsontwin decode|encode.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type User struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Score  float64  `json:"score"`
	Active bool     `json:"active"`
	Tags   []string `json:"tags"`
}

const docs = 1000
const rounds = 100

func users() []User {
	us := make([]User, 0, docs)
	seed := 12345
	for i := 0; i < docs; i++ {
		seed = (seed*1103515245 + 12345) % 2147483648
		n := seed % 100000
		var tags []string
		for t := 0; t < seed%5; t++ {
			tags = append(tags, "t"+strconv.Itoa(t))
		}
		if tags == nil {
			tags = []string{}
		}
		us = append(us, User{ID: n, Name: "user " + strconv.Itoa(n), Email: "user_" + strconv.Itoa(n) + "@example.com", Score: float64(seed%1000) + 0.5, Active: seed%2 == 0, Tags: tags})
	}
	return us
}

// text builds the document by hand, not with the encoder under test.
func text(u User) string {
	var b strings.Builder
	b.WriteString(`{"id":` + strconv.Itoa(u.ID) + `,"name":"` + u.Name + `","email":"` + u.Email + `","score":` + strconv.FormatFloat(u.Score, 'f', 1, 64) + `,"active":` + strconv.FormatBool(u.Active) + `,"tags":[`)
	for i, t := range u.Tags {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"` + t + `"`)
	}
	b.WriteString(`]}`)
	return b.String()
}

func main() {
	w := "decode"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	us := users()
	h := 0
	switch w {
	case "decode":
		ds := make([][]byte, len(us))
		for i, u := range us {
			ds[i] = []byte(text(u))
		}
		for r := 0; r < rounds; r++ {
			for _, d := range ds {
				var u User
				if err := json.Unmarshal(d, &u); err != nil {
					panic(err)
				}
				a := 0
				if u.Active {
					a = 1
				}
				h = (h*31 + u.ID + len(u.Name) + len(u.Email) + len(u.Tags) + int(u.Score)%100 + a) % 1000000007
			}
		}
	case "encode":
		for r := 0; r < rounds; r++ {
			for _, u := range us {
				b, err := json.Marshal(u)
				if err != nil {
					panic(err)
				}
				h = (h*31 + len(b)) % 1000000007
			}
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown workload", w)
		os.Exit(2)
	}
	fmt.Println(w, h)
}
