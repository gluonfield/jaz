package drive

import (
	"fmt"
	"regexp"
	"strings"
)

var queryToken = regexp.MustCompile(`'(?:\\.|[^'\\])*'|[A-Za-z][A-Za-z0-9]*|!=|<=|>=|[^\s]`)

func searchQuery(input string) (string, error) {
	tokens := queryToken.FindAllString(strings.TrimSpace(input), -1)
	var out []string
	for i := 0; i < len(tokens); i++ {
		term := tokens[i]
		switch term {
		case "title":
			term = "name"
		case "parentId", "owner", "sharedWithMe":
			if i+2 >= len(tokens) || (tokens[i+1] != "=" && tokens[i+1] != "!=") {
				if term == "sharedWithMe" {
					out = append(out, term)
					continue
				}
				return "", fmt.Errorf("%s requires = or != and a quoted value", term)
			}
			operator, value := tokens[i+1], tokens[i+2]
			i += 2
			if term == "sharedWithMe" {
				if value != "true" && value != "false" {
					return "", fmt.Errorf("sharedWithMe requires true or false")
				}
				if (operator == "=") == (value == "true") {
					out = append(out, term)
				} else {
					out = append(out, "not sharedWithMe")
				}
				continue
			}
			if !strings.HasPrefix(value, "'") {
				return "", fmt.Errorf("%s requires a quoted value", term)
			}
			collection := "parents"
			if term == "owner" {
				collection = "owners"
			}
			clause := value + " in " + collection
			if operator == "!=" {
				clause = "not (" + clause + ")"
			}
			out = append(out, clause)
			continue
		}
		out = append(out, term)
	}
	query := "trashed = false"
	if len(out) > 0 {
		query += " and (" + strings.Join(out, " ") + ")"
	}
	return query, nil
}
