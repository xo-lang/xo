package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type User struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Score  float64  `json:"score"`
	Active bool     `json:"active"`
	Tags   []string `json:"tags"`
}

func mix(h int, s string) int {
	x := (h*31 + len(s)) % 1000000007
	if len(s) > 0 {
		x = (x*31 + int(s[len(s)/2])) % 1000000007
	}
	return x
}

func main() {
	w := "invalid"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	var bad []string
	x := 12345
	for i := 0; i < 5000; i++ {
		x = (x*1103515245 + 12345) % 2147483648
		v := x % 1000000000
		y := v
		for k := 0; k < v%10; k++ {
			y /= 10
		}
		if i < 1000 {
			var name string
			switch i % 3 {
			case 0:
				name = fmt.Sprintf("user %d", v)
			case 1:
				name = fmt.Sprintf("Jörg \\\"%d\\\"", y)
			default:
				name = fmt.Sprintf("name %d", y)
			}
			tags := "[]"
			if i%2 == 0 {
				tags = "[\"a\",\"bb\"]"
			}
			d := fmt.Sprintf("{\"id\": %d, \"name\": \"%s\", \"email\": \"u%d@example.com\", \"score\": %d.5, \"active\": %v, \"tags\": %s}", v, name, y, y, i%2 == 0, tags)
			switch i % 4 {
			case 0:
				bad = append(bad, d+"x")
			case 1:
				bad = append(bad, d[:len(d)/2])
			case 2:
				bad = append(bad, fmt.Sprintf("{\"id\": %d, \"name\" \"x\"}", v))
			default:
				bad = append(bad, fmt.Sprintf("[1, 2, %de]", v))
			}
		}
	}
	h := 0
	if w == "invalid" {
		for r := 0; r < 40; r++ {
			for _, d := range bad {
				var u User
				if err := json.Unmarshal([]byte(d), &u); err != nil {
					h = mix(h, err.Error())
				} else {
					h++
				}
			}
		}
	}
	fmt.Println(w, h)
}
