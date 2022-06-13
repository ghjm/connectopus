package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func errExit(err error) {
	fmt.Printf("Error: %s\n", err)
	os.Exit(1)
}

var header = `package ui_embed

var uiRoutes = []string{
`

var footer = `}
`

func main() {
	f, err := os.Open("../../ui/src/app/routes.tsx")
	if err != nil {
		errExit(err)
	}
	s := bufio.NewScanner(f)
	routes := make([]string, 0)
	inRoutes := false
	for s.Scan() {
		line := s.Text()
		line = strings.TrimSpace(line)
		if inRoutes {
			if strings.HasPrefix(line, "]") {
				break
			}
			if strings.HasPrefix(line, "path:") {
				line = strings.TrimPrefix(line, "path:")
				line = strings.TrimSpace(line)
				line = strings.TrimSuffix(line, ",")
				if !strings.HasPrefix(line, "'") || !strings.HasSuffix(line, "'") {
					errExit(fmt.Errorf("unparseable route: %s", line))
				}
				line = strings.Trim(line, "'")
				if line == "/" {
					continue
				}
				routes = append(routes, line)
			}
		} else if line == "const routes: AppRouteConfig[] = [" {
			inRoutes = true
		}
	}
	if len(routes) == 0 {
		errExit(fmt.Errorf("no routes found"))
	}

	f, err = os.Create("routes_gen.go")
	if err != nil {
		errExit(err)
	}
	_, err = f.WriteString(header)
	if err != nil {
		errExit(err)
	}
	for _, route := range routes {
		_, err = f.WriteString(fmt.Sprintf("	\"%s\",\n", route))
		if err != nil {
			errExit(err)
		}
	}
	_, err = f.WriteString(footer)
	if err != nil {
		errExit(err)
	}
}
